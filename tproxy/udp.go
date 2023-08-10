package tproxy

import (
	"context"
	"fmt"
	"net"
	"time"
)

type UDPProxy struct {
	listener    net.Listener
	mapper      Mapper
	baseCtx     context.Context
	dialer      Dialer
	dialTimeout time.Duration
}

func NewUDPProxy(ctx context.Context, cfg *Config) (*UDPProxy, error) {
	cfg.populateDefaults()

	listenConfig := net.ListenConfig{
		Control: transparentDgramControlFunc,
	}

	listener, err := listenConfig.Listen(ctx, "udp", cfg.ListenAddr.String())
	if err != nil {
		return nil, fmt.Errorf("unable to start UDP proxy listener: %w", err)
	}

	proxy := &UDPProxy{
		listener:    listener,
		mapper:      cfg.Mapper,
		baseCtx:     ctx,
		dialer:      cfg.Dialer,
		dialTimeout: cfg.DialTimeout,
	}
	go proxy.listen()

	return proxy, nil
}

func (t *UDPProxy) listen() {
}
