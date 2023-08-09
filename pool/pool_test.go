package pool

import (
	"net/netip"
	"testing"
)

func TestComplex(t *testing.T) {
	start := netip.MustParseAddr("172.24.0.0")
	end := netip.MustParseAddr("172.24.255.255")
	p, err := New(start, end)
	if err != nil {
		t.Fatalf("can't create IP pool: %v", err)
	}

	ips := make(map[netip.Addr]struct{})
	for i := 0; i < 1000; i++ {
		ip := p.GetRandom()
		if ip.Less(start) || end.Less(ip) {
			t.Fatalf("IP %s is outside pool range (%s-%s)", ip, start, end)
		}

		ips[ip] = struct{}{}
	}

	if len(ips) < 500 {
		t.Fatalf("too few different addresses returned: %d", len(ips))
	}
}
