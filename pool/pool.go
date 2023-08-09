package pool

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"net/netip"

	"github.com/Snawoot/dns44/utils/random"
)

type addressPoolV4 struct {
	base uint32
	size uint32
	rng  *rand.Rand
}

type AddressPool interface {
	GetRandom() netip.Addr
}

var (
	ErrUnsupportedAddressFamily = errors.New("unsupported address family")
	ErrBadOrder                 = errors.New("end of range is less than start of range")
)

func New(start, end netip.Addr) (AddressPool, error) {
	if !start.Is4() || !end.Is4() {
		return nil, ErrUnsupportedAddressFamily
	}
	if end.Less(start) {
		return nil, ErrBadOrder
	}

	base := binary.BigEndian.Uint32(start.AsSlice())
	return &addressPoolV4{
		base: base,
		size: binary.BigEndian.Uint32(end.AsSlice()) - base + 1,
		rng:  random.NewTimeSeededRand(),
	}, nil
}

func (p *addressPoolV4) GetRandom() netip.Addr {
	ip := p.base + uint32(p.rng.Intn(int(p.size)))
	ipSlice := make([]byte, 4)
	binary.BigEndian.PutUint32(ipSlice, ip)
	res, ok := netip.AddrFromSlice(ipSlice)
	if !ok {
		panic(errors.New("unexpected return from netip.AddrFromSlice()"))
	}
	return res
}
