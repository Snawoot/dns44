package tproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// UDPConnTrackTimeout is the timeout used for UDP connection tracking
	UDPConnTrackTimeout = 90 * time.Second
	// UDPBufSize is the buffer size for the UDP proxy
	UDPBufSize = 65507
)

// A net.Addr where the IP is split into two fields so you can use it as a key
// in a map:
type connTrackKey struct {
	IPHigh uint64
	IPLow  uint64
	Port   int
}

func newConnTrackKey(addr *net.UDPAddr) *connTrackKey {
	if len(addr.IP) == net.IPv4len {
		return &connTrackKey{
			IPHigh: 0,
			IPLow:  uint64(binary.BigEndian.Uint32(addr.IP)),
			Port:   addr.Port,
		}
	}
	return &connTrackKey{
		IPHigh: binary.BigEndian.Uint64(addr.IP[:8]),
		IPLow:  binary.BigEndian.Uint64(addr.IP[8:]),
		Port:   addr.Port,
	}
}

type connTrackMap map[connTrackKey]net.Conn

type UDPProxy struct {
	listener       *net.UDPConn
	mapper         Mapper
	baseCtx        context.Context
	dialer         Dialer
	dialTimeout    time.Duration
	connTrackTable connTrackMap
	connTrackLock  sync.Mutex
}

func NewUDPProxy(ctx context.Context, cfg *Config) (*UDPProxy, error) {
	cfg.populateDefaults()

	listenConfig := net.ListenConfig{
		Control: transparentDgramControlFunc,
	}

	listener, err := listenConfig.ListenPacket(ctx, "udp", cfg.ListenAddr.String())
	if err != nil {
		return nil, fmt.Errorf("unable to start UDP proxy listener: %w", err)
	}
	udpListener, ok := listener.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("unable to assert listener type")
	}

	proxy := &UDPProxy{
		listener:    udpListener,
		mapper:      cfg.Mapper,
		baseCtx:     ctx,
		dialer:      cfg.Dialer,
		dialTimeout: cfg.DialTimeout,
	}

	return proxy, nil
}

func (proxy *UDPProxy) replyLoop(proxyConn net.Conn, clientAddr *net.UDPAddr, clientKey *connTrackKey) {
	defer func() {
		proxy.connTrackLock.Lock()
		delete(proxy.connTrackTable, *clientKey)
		proxy.connTrackLock.Unlock()
		proxyConn.Close()
	}()

	readBuf := make([]byte, UDPBufSize)
	for {
		proxyConn.SetReadDeadline(time.Now().Add(UDPConnTrackTimeout))
	again:
		read, err := proxyConn.Read(readBuf)
		if err != nil {
			if err, ok := err.(*net.OpError); ok && err.Err == syscall.ECONNREFUSED {
				// This will happen if the last write failed
				// (e.g: nothing is actually listening on the
				// proxied port on the container), ignore it
				// and continue until UDPConnTrackTimeout
				// expires:
				goto again
			}
			return
		}
		for i := 0; i != read; {
			written, err := proxy.listener.WriteToUDP(readBuf[i:read], clientAddr)
			if err != nil {
				return
			}
			i += written
		}
	}
}

// Run starts forwarding the traffic using UDP.
func (proxy *UDPProxy) Run() {
	proxy.connTrackTable = make(connTrackMap)
	readBuf := make([]byte, UDPBufSize)
	for {
		read, from, to, err := ReadFromUDP(proxy.listener, readBuf)
		if err != nil {
			// NOTE: Apparently ReadFrom doesn't return
			// ECONNREFUSED like Read do (see comment in
			// UDPProxy.replyLoop)
			if !isClosedError(err) {
				log.Printf("stopping proxy on udp: %v", err)
			}
			break
		}

		fromKey := newConnTrackKey(from)
		proxy.connTrackLock.Lock()
		proxyConn, hit := proxy.connTrackTable[*fromKey]
		if !hit {
			proxyConn, err := proxy.makeOutboundConn(from.AddrPort(), to.AddrPort())
			if err != nil {
				log.Printf("can't proxy a datagram to udp: %v", err)
				proxy.connTrackLock.Unlock()
				continue
			}
			proxy.connTrackTable[*fromKey] = proxyConn
			go proxy.replyLoop(proxyConn, from, fromKey)
		}
		proxy.connTrackLock.Unlock()
		for i := 0; i != read; {
			written, err := proxyConn.Write(readBuf[i:read])
			if err != nil {
				log.Printf("can't proxy a datagram to udp: %v", err)
				break
			}
			i += written
		}
	}
}

func (proxy *UDPProxy) makeOutboundConn(from, to netip.AddrPort) (net.Conn, error) {
	// TODO: implement deferred Dial so it won't block socket recv loop
	domainName, ok, err := proxy.mapper.ReverseLookup(from.Addr().String(), to.Addr())
	if err != nil {
		return nil, fmt.Errorf("reverse lookup in UDP handler failed: %w", err)
	}

	if !ok {
		return nil, fmt.Errorf("reverse mapping not found for address (%s=>%s)", from.Addr().String(), to.Addr().String())
	}

	if domainName == "" {
		return nil, fmt.Errorf("bad domain name for address (%s=>%s)", from.Addr().String(), to.Addr().String())
	}

	log.Printf("[+] UDP %s <=> [%s(%s)]:%d", from.String(), domainName, to.Addr().String(), to.Port())

	dialAddress := net.JoinHostPort(domainName, strconv.FormatUint(uint64(to.Port()), 10))
	dialCtx, cancel := context.WithTimeout(context.Background(), proxy.dialTimeout)
	defer cancel()

	conn, err := proxy.dialer.DialContext(dialCtx, "udp", dialAddress)
	if err != nil {
		return nil, fmt.Errorf("remote dial failed: %w", err)
	}

	return conn, nil
}

// Close stops forwarding the traffic.
func (proxy *UDPProxy) Close() {
	proxy.listener.Close()
	proxy.connTrackLock.Lock()
	defer proxy.connTrackLock.Unlock()
	for _, conn := range proxy.connTrackTable {
		conn.Close()
	}
}

func isClosedError(err error) bool {
	/* This comparison is ugly, but unfortunately, net.go doesn't export errClosing.
	 * See:
	 * http://golang.org/src/pkg/net/net.go
	 * https://code.google.com/p/go/issues/detail?id=4337
	 * https://groups.google.com/forum/#!msg/golang-nuts/0_aaCvBmOcM/SptmDyX1XJMJ
	 */
	return strings.HasSuffix(err.Error(), "use of closed network connection")
}
