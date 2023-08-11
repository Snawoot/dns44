package tproxy

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const defFutureConnBacklog = 256

var errBacklogOverflow = errors.New("backlog overflow")

type futureConn struct {
	conn       net.Conn
	connErr    error
	connCh     chan struct{}
	backlogMux sync.RWMutex
	backlog    chan []byte
	doWrites   bool
}

func newFutureConn(dial func() (net.Conn, error), backlog int) *futureConn {
	if backlog < 1 {
		backlog = defFutureConnBacklog
	}
	c := &futureConn{
		connCh:  make(chan struct{}),
		backlog: make(chan []byte, backlog),
	}

	go c.bgDial(dial)

	return c
}

func (c *futureConn) Read(b []byte) (n int, err error) {
	c.WaitResolve()
	return c.conn.Read(b)
}

func (c *futureConn) Close() error {
	c.WaitResolve()
	if c.connErr != nil {
		return nil
	}
	return c.conn.Close()
}

func (c *futureConn) LocalAddr() net.Addr {
	c.WaitResolve()
	if c.connErr != nil {
		return nil
	}
	return c.conn.LocalAddr()
}

func (c *futureConn) RemoteAddr() net.Addr {
	c.WaitResolve()
	if c.connErr != nil {
		return nil
	}
	return c.conn.LocalAddr()
}

func (c *futureConn) SetDeadline(t time.Time) error {
	c.WaitResolve()
	if c.connErr != nil {
		return fmt.Errorf("postponed error: %w", c.connErr)
	}
	return c.conn.SetDeadline(t)
}

func (c *futureConn) SetReadDeadline(t time.Time) error {
	c.WaitResolve()
	if c.connErr != nil {
		return fmt.Errorf("postponed error: %w", c.connErr)
	}
	return c.conn.SetReadDeadline(t)
}

func (c *futureConn) SetWriteDeadline(t time.Time) error {
	c.WaitResolve()
	if c.connErr != nil {
		return fmt.Errorf("postponed error: %w", c.connErr)
	}
	return c.conn.SetWriteDeadline(t)
}

func (c *futureConn) WaitResolve() {
	<-c.connCh
}

func (c *futureConn) Write(b []byte) (int, error) {
	c.backlogMux.RLock()
	defer c.backlogMux.RUnlock()

	if c.doWrites {
		return c.conn.Write(b)
	}

	n := len(b)
	stored := make([]byte, n)
	copy(stored, b)

	select {
	case c.backlog <- stored:
		return n, nil
	default:
		return 0, errBacklogOverflow
	}
}

func (c *futureConn) bgDial(dial func() (net.Conn, error)) {
	c.conn, c.connErr = dial()
	if c.connErr != nil {
		log.Printf("bgDial: dial failed: %v", c.connErr)
		return
	}
	close(c.connCh)

	c.backlogMux.Lock()
	defer c.backlogMux.Unlock()

	c.doWrites = true
	close(c.backlog)

	for buf := range c.backlog {
		_, err := c.conn.Write(buf)
		if err != nil {
			log.Printf("bgDial: postponed write failed: %v", err)
		}
	}
}
