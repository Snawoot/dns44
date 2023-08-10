package tproxy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"unsafe"
)

var nativeEndian binary.ByteOrder

func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)

	switch buf {
	case [2]byte{0xCD, 0xAB}:
		nativeEndian = binary.LittleEndian
	case [2]byte{0xAB, 0xCD}:
		nativeEndian = binary.BigEndian
	default:
		panic("Could not determine native endianness.")
	}
}

const (
	IPV6_TRANSPARENT     = 75
	IPV6_RECVORIGDSTADDR = 74
)

func transparentControlFunc(network, address string, conn syscall.RawConn) error {
	var operr error
	if err := conn.Control(func(fd uintptr) {
		level := syscall.SOL_IP
		optname := syscall.IP_TRANSPARENT
		switch network {
		case "tcp6", "udp6", "ip6":
			level = syscall.SOL_IPV6
			optname = IPV6_TRANSPARENT
		}
		operr = syscall.SetsockoptInt(int(fd), level, optname, 1)
	}); err != nil {
		return err
	}
	return operr
}

func transparentDgramControlFunc(network, address string, conn syscall.RawConn) error {
	var operr error
	if err := conn.Control(func(fd uintptr) {
		level := syscall.SOL_IP
		transOptName := syscall.IP_TRANSPARENT
		origDstOptName := syscall.IP_RECVORIGDSTADDR
		switch network {
		case "tcp6", "udp6", "ip6":
			level = syscall.SOL_IPV6
			transOptName = IPV6_TRANSPARENT
			origDstOptName = IPV6_RECVORIGDSTADDR
		}

		operr = syscall.SetsockoptInt(int(fd), level, transOptName, 1)
		if operr != nil {
			return
		}
		operr = syscall.SetsockoptInt(int(fd), level, origDstOptName, 1)
	}); err != nil {
		return err
	}
	return operr
}

// ReadFromUDP reads a UDP packet from c, copying the payload into b.
// It returns the number of bytes copied into b and the return address
// that was on the packet.
//
// Out-of-band data is also read in so that the original destination
// address can be identified and parsed.
func ReadFromUDP(conn *net.UDPConn, b []byte) (int, *net.UDPAddr, *net.UDPAddr, error) {
	oob := make([]byte, 1024)
	n, oobn, _, addr, err := conn.ReadMsgUDP(b, oob)
	if err != nil {
		return 0, nil, nil, err
	}

	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("parsing socket control message: %s", err)
	}

	ntohs := func(n uint16) uint16 {
		return (n >> 8) | (n << 8)
	}

	var originalDst *net.UDPAddr
	for _, msg := range msgs {
		if msg.Header.Level == syscall.SOL_IP && msg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			originalDstRaw := &syscall.RawSockaddrInet4{}
			if err = binary.Read(bytes.NewReader(msg.Data), nativeEndian, originalDstRaw); err != nil {
				return 0, nil, nil, fmt.Errorf("reading original destination address: %s", err)
			}
			originalDst = &net.UDPAddr{
				IP:   net.IPv4(originalDstRaw.Addr[0], originalDstRaw.Addr[1], originalDstRaw.Addr[2], originalDstRaw.Addr[3]),
				Port: int(ntohs(originalDstRaw.Port)),
			}
		} else if msg.Header.Level == syscall.SOL_IPV6 && msg.Header.Type == IPV6_RECVORIGDSTADDR {
			originalDstRaw := &syscall.RawSockaddrInet6{}
			if err = binary.Read(bytes.NewReader(msg.Data), nativeEndian, originalDstRaw); err != nil {
				return 0, nil, nil, fmt.Errorf("reading original destination address: %s", err)
			}
			originalDst = &net.UDPAddr{
				IP:   originalDstRaw.Addr[:],
				Port: int(ntohs(originalDstRaw.Port)),
				Zone: strconv.Itoa(int(originalDstRaw.Scope_id)),
			}
		}
	}

	if originalDst == nil {
		return 0, nil, nil, fmt.Errorf("unable to obtain original destination: %s", err)
	}

	return n, addr, originalDst, nil
}
