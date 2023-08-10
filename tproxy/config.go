package tproxy

import (
	"net"
	"net/netip"
	"time"
)

const (
	DefaultDialTimeout = 10 * time.Second
)

type Config struct {
	ListenAddr  netip.AddrPort
	Mapper      Mapper
	DialTimeout time.Duration
	Dialer      Dialer
}

func (cfg *Config) populateDefaults() {
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = DefaultDialTimeout
	}
	if cfg.Dialer == nil {
		cfg.Dialer = new(net.Dialer)
	}
}
