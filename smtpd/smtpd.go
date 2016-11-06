package smtpd

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"time"
)

type Connection interface {
	Printf(format string, args ...interface{}) error
	Reply(code int, messages ...string) error
	StartTLS(*tls.Config)
	ReadCommand(timeout int) (command, args string, err error)
	ReadDotBytes(timeout int) ([]byte, error)
	Close() error
	RemoteAddr() net.Addr
	Tarpit() (int, time.Duration, error)
}

type NetConnection struct {
	conn   net.Conn
	reader *textproto.Reader
}

func NewConnection(conn net.Conn) Connection {
	return &NetConnection{
		conn:   conn,
		reader: textproto.NewReader(bufio.NewReader(conn)),
	}
}

func (c *NetConnection) Printf(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(c.conn, format, args...)
	return err
}

func (c *NetConnection) Reply(code int, messages ...string) error {
	for i, text := range messages {
		sep := " "
		if i < len(messages)-1 {
			sep = "-"
		}
		if err := c.Printf("%03d%s%s\r\n", code, sep, text); err != nil {
			return err
		}
	}
	return nil
}

func (c *NetConnection) StartTLS(cfg *tls.Config) {
	c.conn = tls.Server(c.conn, cfg)
	c.reader = textproto.NewReader(bufio.NewReader(c.conn))
}

func (c *NetConnection) ReadCommand(timeout int) (command, args string, err error) {
	c.conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
	line, err := c.reader.ReadLine()
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 1 {
		return parts[0], "", nil
	} else {
		return parts[0], parts[1], nil
	}
}

func (c *NetConnection) ReadDotBytes(timeout int) ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
	return c.reader.ReadDotBytes()
}

func (c *NetConnection) Close() error {
	return c.conn.Close()
}

func (c *NetConnection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *NetConnection) Tarpit() (int, time.Duration, error) {
	c.conn.SetReadDeadline(time.Time{})
	buf := make([]byte, 1024, 1024)
	bytes := 0
	start := time.Now()
	for {
		n, err := c.conn.Read(buf)
		bytes += n
		if err != nil {
			return bytes, time.Now().Sub(start), err
		}
	}
}
