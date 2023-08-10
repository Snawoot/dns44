package tproxy

import (
	"context"
	"net"
	"net/netip"
)

type Mapper interface {
	ReverseLookup(clientKey string, addr netip.Addr) (domainName string, ok bool, err error)
}

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}
