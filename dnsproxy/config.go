package dnsproxy

import (
	"net/netip"
	"time"
)

type Mapper interface {
	EnsureMapping(clientKey, domainName string, ttl time.Duration) (netip.Addr, error)
}

// Config is the DNS proxy configuration.
type Config struct {
	// ListenAddr is the address the DNS server is supposed to listen to.
	ListenAddr netip.AddrPort

	// Upstream is the upstream that the requests will be forwarded to.  The
	// format of an upstream is the one that can be consumed by
	// [proxy.ParseUpstreamsConfig].
	Upstream string

	// Mapper is the database which grants one to one mapping between domain and network address
	Mapper Mapper
}
