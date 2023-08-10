package tproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"time"
)

type TCPProxy struct {
	listener    net.Listener
	mapper      Mapper
	baseCtx     context.Context
	dialer      Dialer
	dialTimeout time.Duration
}

func NewTCPProxy(ctx context.Context, cfg *Config) (*TCPProxy, error) {
	cfg.populateDefaults()

	listenConfig := net.ListenConfig{
		Control: transparentControlFunc,
	}

	listener, err := listenConfig.Listen(ctx, "tcp", cfg.ListenAddr.String())
	if err != nil {
		return nil, fmt.Errorf("unable to start TCP proxy listener: %w", err)
	}

	proxy := &TCPProxy{
		listener:    listener,
		mapper:      cfg.Mapper,
		baseCtx:     ctx,
		dialer:      cfg.Dialer,
		dialTimeout: cfg.DialTimeout,
	}
	go proxy.listen()

	return proxy, nil
}

func (t *TCPProxy) listen() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("temporary error while accepting connection: %s", netErr)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			select {
			case <-t.baseCtx.Done():
			default:
				log.Printf("unrecoverable error while accepting connection: %s", err)
			}
			return
		}

		go t.handle(conn)
	}
}

func (t *TCPProxy) handle(conn net.Conn) {
	log.Printf("accept: TCP %s <=> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	defer conn.Close()

	rAddr, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		log.Printf("can't parse remote address: %v", err)
		return
	}
	lAddr, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		log.Printf("can't parse local address: %v", err)
		return
	}

	domainName, ok, err := t.mapper.ReverseLookup(rAddr.Addr().String(), lAddr.Addr())
	if err != nil {
		log.Printf("reverse lookup in TCP handler failed: %v", err)
		return
	}

	if !ok {
		log.Printf("reverse mapping not found for address %s", lAddr.Addr().String())
		return
	}

	if domainName == "" {
		log.Printf("bad domain name for address %s", lAddr.Addr().String())
		return
	}

	dialAddress := net.JoinHostPort(domainName, strconv.FormatUint(uint64(lAddr.Port()), 10))
	dialCtx, cancel := context.WithTimeout(t.baseCtx, t.dialTimeout)
	defer cancel()

	upstreamConn, err := t.dialer.DialContext(dialCtx, "tcp", dialAddress)
	if err != nil {
		log.Printf("remote dial failed: %v", err)
		return
	}
	defer upstreamConn.Close()

	conn.Write([]byte("Hello, World!\n"))
}
