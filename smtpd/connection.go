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

type Conn struct {
	name     string
	netConn  net.Conn
	tpReader *textproto.Reader
	isTLS    bool
}

func NewConn(conn net.Conn) *Conn {
	br := bufio.NewReader(conn)
	tr := textproto.NewReader(br)
	return &Conn{
		name:     conn.RemoteAddr().String(),
		netConn:  conn,
		tpReader: tr,
	}
}

func (c *Conn) ReadLine(duration string) (string, error) {
	c.netConn.SetReadDeadline(parseDuration(duration))
	defer c.netConn.SetReadDeadline(time.Time{})
	return c.tpReader.ReadLine()
}

func (c *Conn) ReadCommand(duration string) (command, args string, err error) {
	line, err := c.ReadLine(duration)
	if err != nil {
		return "", "", err
	}
	strings := strings.SplitN(line, " ", 2)
	if len(strings) == 1 {
		return strings[0], "", nil
	} else {
		return strings[0], strings[1], nil
	}
}

func (c *Conn) Reply(code int, messages ...string) error {
	for i, text := range messages {
		sep := " "
		if i < len(messages)-1 {
			sep = "-"
		}
		line := fmt.Sprintf("%03d%s%s", code, sep, text)
		err := c.WriteLine(line)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) WriteLine(message string) error {
	buffer := make([]byte, len(message)+2)
	copy(buffer, message)
	copy(buffer[len(message):], "\r\n")
	_, err := c.netConn.Write(buffer)
	return err
}

func (c *Conn) Close() error {
	return c.netConn.Close()
}

func (c *Conn) IsTLS() bool {
	return c.isTLS
}

func (c *Conn) StartTLS(tlsConfig *tls.Config) {
	conn := tls.Server(c.netConn, tlsConfig)
	br := bufio.NewReader(conn)
	tr := textproto.NewReader(br)
	c.netConn = conn
	c.tpReader = tr
	c.isTLS = true
}

func (c *Conn) ReadDotBytes() ([]byte, error) {
	return c.tpReader.ReadDotBytes()
}

func (c *Conn) String() string {
	return c.name
}

func parseDuration(duration string) time.Time {
	delay, err := time.ParseDuration(duration)
	if err != nil {
		panic(err)
	}
	return time.Now().Add(delay)
}
