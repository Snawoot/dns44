package tproxy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
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

// DialUDP connects to the remote address raddr on the network net,
// which must be "udp", "udp4", or "udp6".  If laddr is not nil, it is
// used as the local address for the connection.
func DialUDP(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (*net.UDPConn, error) {
	remoteSocketAddress, err := udpAddrToSocketAddr(raddr)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("build destination socket address: %s", err)}
	}

	localSocketAddress, err := udpAddrToSocketAddr(laddr)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("build local socket address: %s", err)}
	}

	fileDescriptor, err := syscall.Socket(udpAddrFamily(network, laddr, raddr), syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("socket open: %s", err)}
	}

	if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fileDescriptor)
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("set socket option: SO_REUSEADDR: %s", err)}
	}

	if laddr.IP.To4() != nil {
		if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
			syscall.Close(fileDescriptor)
			return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("set socket option: IP_TRANSPARENT: %s", err)}
		}
	} else {
		if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IPV6, IPV6_TRANSPARENT, 1); err != nil {
			syscall.Close(fileDescriptor)
			return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("set socket option: IPV6_TRANSPARENT: %s", err)}
		}
	}

	if err = syscall.Bind(fileDescriptor, localSocketAddress); err != nil {
		syscall.Close(fileDescriptor)
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("socket bind: %s", err)}
	}

	if err = syscall.Connect(fileDescriptor, remoteSocketAddress); err != nil {
		syscall.Close(fileDescriptor)
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("socket connect: %s", err)}
	}

	fdFile := os.NewFile(uintptr(fileDescriptor), fmt.Sprintf("net-udp-dial-%s", raddr.String()))
	defer fdFile.Close()

	remoteConn, err := net.FileConn(fdFile)
	if err != nil {
		syscall.Close(fileDescriptor)
		return nil, &net.OpError{Op: "dial", Err: fmt.Errorf("convert file descriptor to connection: %s", err)}
	}

	return remoteConn.(*net.UDPConn), nil
}

// udpAddToSockerAddr will convert a UDPAddr
// into a Sockaddr that may be used when
// connecting and binding sockets
func udpAddrToSocketAddr(addr *net.UDPAddr) (syscall.Sockaddr, error) {
	switch {
	case addr.IP.To4() != nil:
		ip := [4]byte{}
		copy(ip[:], addr.IP.To4())

		return &syscall.SockaddrInet4{Addr: ip, Port: addr.Port}, nil

	default:
		ip := [16]byte{}
		copy(ip[:], addr.IP.To16())

		zoneID, err := strconv.ParseUint(addr.Zone, 10, 32)
		if err != nil {
			zoneID = 0
		}

		return &syscall.SockaddrInet6{Addr: ip, Port: addr.Port, ZoneId: uint32(zoneID)}, nil
	}
}

// udpAddrFamily will attempt to work
// out the address family based on the
// network and UDP addresses
func udpAddrFamily(net string, laddr, raddr *net.UDPAddr) int {
	switch net[len(net)-1] {
	case '4':
		return syscall.AF_INET
	case '6':
		return syscall.AF_INET6
	}

	if (laddr == nil || laddr.IP.To4() != nil) &&
		(raddr == nil || laddr.IP.To4() != nil) {
		return syscall.AF_INET
	}
	return syscall.AF_INET6
}
