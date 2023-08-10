package tproxy

import "syscall"

func transparentControlFunc(network, address string, conn syscall.RawConn) error {
	var operr error
	if err := conn.Control(func(fd uintptr) {
		operr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	}); err != nil {
		return err
	}
	return operr
}
