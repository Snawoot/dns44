package tproxy

import "syscall"

func transparentControlFunc(network, address string, conn syscall.RawConn) error {
	var operr error
	if err := conn.Control(func(fd uintptr) {
		level := syscall.SOL_IP
		switch network {
		case "tcp6", "udp6", "ip6":
			level = syscall.SOL_IPV6
		}
		operr = syscall.SetsockoptInt(int(fd), level, syscall.IP_TRANSPARENT, 1)
	}); err != nil {
		return err
	}
	return operr
}

func transparentDgramControlFunc(network, address string, conn syscall.RawConn) error {
	var operr error
	if err := conn.Control(func(fd uintptr) {
		level := syscall.SOL_IP
		switch network {
		case "tcp6", "udp6", "ip6":
			level = syscall.SOL_IPV6
		}

		operr = syscall.SetsockoptInt(int(fd), level, syscall.IP_TRANSPARENT, 1)
		if operr != nil {
			return
		}
		operr = syscall.SetsockoptInt(int(fd), level, syscall.IP_RECVORIGDSTADDR, 1)
	}); err != nil {
		return err
	}
	return operr
}
