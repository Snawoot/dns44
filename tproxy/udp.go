package tproxy

import (
	"context"
	"fmt"
	"io"
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
	from netip.AddrPort
	to   netip.AddrPort
}

func (key connTrackKey) String() string {
	return fmt.Sprintf("<%s,%s>", key.from.String(), key.to.String())
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

	go proxy.listen()

	return proxy, nil
}

func (proxy *UDPProxy) replyLoop(proxyConn net.Conn, clientAddr *net.UDPAddr, localAddr *net.UDPAddr, ctKey connTrackKey) {
	defer func() {
		proxy.connTrackLock.Lock()
		delete(proxy.connTrackTable, ctKey)
		proxy.connTrackLock.Unlock()
		proxyConn.Close()
		log.Printf("[-] UDP %s <=> %s", ctKey.from.String(), ctKey.to.String())
	}()

	respConn, err := DialUDP("udp", localAddr, clientAddr)
	if err != nil {
		log.Printf("unable to open reply UDP connection: %v", err)
	}
	defer respConn.Close()
	go io.Copy(proxyConn, respConn)

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
			log.Printf("reply loop (%s) stopped on read for reason: %v", ctKey.String(), err)
			return
		}
		_, err = respConn.Write(readBuf[:read])
		if err != nil {
			log.Printf("reply loop (%s) stopped on write for reason: %v", ctKey.String(), err)
			return
		}
	}
}

// listen starts forwarding the traffic using UDP.
func (proxy *UDPProxy) listen() {
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
		from, to = unmapUDPAddr(from), unmapUDPAddr(to)

		ctKey := connTrackKey{from.AddrPort(), to.AddrPort()}
		proxy.connTrackLock.Lock()
		proxyConn, hit := proxy.connTrackTable[ctKey]
		if !hit {
			proxyConn, err = proxy.makeOutboundConn(from.AddrPort(), to.AddrPort())
			if err != nil {
				log.Printf("can't proxy a datagram to udp: %v", err)
				proxy.connTrackLock.Unlock()
				continue
			}
			proxy.connTrackTable[ctKey] = proxyConn
			go proxy.replyLoop(proxyConn, from, to, ctKey)
		}
		proxy.connTrackLock.Unlock()
		_, err = proxyConn.Write(readBuf[:read])
		if err != nil {
			log.Printf("can't proxy a datagram to udp: %v", err)
		}
	}
}

func (proxy *UDPProxy) makeOutboundConn(from, to netip.AddrPort) (net.Conn, error) {
	futureConn := newFutureConn(func() (net.Conn, error) {
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
		dialCtx, cancel := context.WithTimeout(proxy.baseCtx, proxy.dialTimeout)
		defer cancel()

		conn, err := proxy.dialer.DialContext(dialCtx, "udp", dialAddress)
		if err != nil {
			return nil, fmt.Errorf("remote dial failed: %w", err)
		}

		return conn, nil
	}, 0)

	return futureConn, nil
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

func unmapUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	ap := a.AddrPort()
	apu := netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
	return net.UDPAddrFromAddrPort(apu)
}
