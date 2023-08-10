package tproxy

import "net/netip"

type Mapper interface {
	ReverseLookup(clientKey string, addr netip.Addr) (domainName string, ok bool, err error)
}
