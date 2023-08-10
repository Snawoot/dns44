package tproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"
)

type TCPProxy struct {
	listener net.Listener
	mapper   Mapper
	baseCtx  context.Context
}

func NewTCPProxy(ctx context.Context, listenAddr netip.AddrPort, mapper Mapper) (*TCPProxy, error) {
	listenConfig := net.ListenConfig{
		Control: transparentControlFunc,
	}

	listener, err := listenConfig.Listen(ctx, "tcp", listenAddr.String())
	if err != nil {
		return nil, fmt.Errorf("unable to start TCP proxy listener: %w", err)
	}

	proxy := &TCPProxy{
		listener: listener,
		mapper:   mapper,
		baseCtx:  ctx,
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
	log.Printf("accepting TCP connection from %s with destination of %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	defer conn.Close()

	conn.Write([]byte("Hello, World!\n"))
}
