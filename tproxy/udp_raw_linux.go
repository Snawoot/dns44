package tproxy

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type RawUDPConn struct {
	conn net.PacketConn
}

var (
	ErrUnsupportedAF     = errors.New("unsupported address family")
	ErrUnsupportedMethod = errors.New("unsupported method")
)

func NewRawUDPConn(network string) (*RawUDPConn, error) {
	switch network {
	case "udp4":
	default:
		return nil, ErrUnsupportedAF
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed open socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW): %s", err)
	}
	syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)

	conn, err := net.FilePacketConn(os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd)))
	if err != nil {
		return nil, err
	}

	return &RawUDPConn{
		conn: conn,
	}, nil
}

func buildUDPPacket(b []byte, src, dst *net.UDPAddr) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	payload := gopacket.Payload(b)
	ip := &layers.IPv4{
		DstIP:    dst.IP,
		SrcIP:    src.IP,
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(src.Port),
		DstPort: layers.UDPPort(dst.Port),
	}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		return nil, fmt.Errorf("failed calc checksum: %s", err)
	}
	if err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, ip, udp, payload); err != nil {
		return nil, fmt.Errorf("failed serialize packet: %s", err)
	}
	return buffer.Bytes(), nil
}

func (c *RawUDPConn) WriteFromTo(b []byte, from *net.UDPAddr, to *net.UDPAddr) (int, error) {
	b, err := buildUDPPacket(b, from, to)
	if err != nil {
		return 0, fmt.Errorf("can't build UDP packet: %w", err)
	}
	return c.conn.WriteTo(b, &net.IPAddr{IP: to.IP})
}

func (c *RawUDPConn) Close() error {
	return c.conn.Close()
}
