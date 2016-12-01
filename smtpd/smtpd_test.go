package smtpd

import (
	"bytes"
	"io"
	"math"
	"net"
	"testing"
	"time"
)

func TestNewConnection(t *testing.T) {
	var netconn net.Conn = newFakeConnection()
	if _, ok := NewConnection(netconn).(Connection); !ok {
		t.Error("Expected NewConnection to return a Connection, but it did not")
	}
}

func TestPrintf(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	c.Printf("Hello, %s", "World!")
	expectStringEqual(t, netconn.String(), "Hello, World!")
}

func TestReply(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	c.Reply(25, "Ok")
	expectStringEqual(t, netconn.String(), "025 Ok\r\n")

	netconn.Reset()

	c.Reply(100, "Yes", "No", "Maybe")
	expectStringEqual(t, netconn.String(), "100-Yes\r\n100-No\r\n100 Maybe\r\n")
}

// FIXME: TestStartTLS

func TestReadCommand(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)
	netconn.WriteString("HELO localhost\r\n")

	command, args, err := c.ReadCommand(23)
	timeout := netconn.ReadDeadline.Sub(time.Now()).Seconds()
	if math.Abs(timeout-23.0) > 0.01 {
		t.Errorf("Expected ReadCommand to set read timeout 23s, but set %#v",
			timeout)
	}
	if err != nil {
		t.Errorf("Expected no error, but got %#v", err)
	}
	expectStringEqual(t, command, "HELO")
	expectStringEqual(t, args, "localhost")

	netconn.Reset()
	netconn.WriteString("HELO\r\n")
	command, args, err = c.ReadCommand(23)
	if err != nil {
		t.Errorf("Expected no error, but got %#v", err)
	}
	expectStringEqual(t, command, "HELO")
	expectStringEqual(t, args, "")

	netconn.Reset()
	command, args, err = c.ReadCommand(23)
	if err != io.EOF {
		t.Errorf("Expected an EOF error, but got %#v", err)
	}

	// Supports limited reader
	netconn.Reset()
	netconn.WriteString("HELO ")
	for i := 0; i < 1024; i++ {
		netconn.WriteString("a")
	}
	netconn.WriteString("\r\n")
	command, args, err = c.ReadCommand(23)
	if err != nil {
		t.Errorf("Expected no error, but got %#v", err)
	}
	expectStringEqual(t, command, "HELO")
	if len(args) != 995 {
		t.Errorf("Expected %d bytes, got %d", 995, len(args))
	}
}

func TestReadDotBytes(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	// Not testing whether textproto.ReadDotBytes() actually works
	c.ReadDotBytes(23)

	timeout := netconn.ReadDeadline.Sub(time.Now()).Seconds()
	if math.Abs(timeout-23.0) > 0.01 {
		t.Errorf("Expected ReadDotBytes to set read timeout 23s, but set %#v",
			timeout)
	}
}

func TestClose(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	c.Close()

	if !netconn.Closed {
		t.Error("Expected Close() to close the underlying connection, but didn't")
	}
}

func TestRemoteAddr(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	expectStringEqual(t, netconn.RemoteAddr().String(), c.RemoteAddr().String())
}

func TestTarpit(t *testing.T) {
	netconn := newFakeConnection()
	c := NewConnection(netconn)

	for i := 0; i < 1024; i++ {
		netconn.WriteString("line\r\n")
	}
	bytes, _, err := c.Tarpit()
	expectIntEqual(t, bytes, 6*1024)
	if err != io.EOF {
		t.Errorf("Expected an EOF error, but got %#v", err)
	}
}

// Helper methods

func expectStringEqual(t *testing.T, actual, expected string) {
	if actual != expected {
		t.Errorf("Expected %#v to be %#v", actual, expected)
	}
}

func expectIntEqual(t *testing.T, actual, expected int) {
	if actual != expected {
		t.Errorf("Expected %#v to be %#v", actual, expected)
	}
}

// Mock net.Conn

type NyetConn struct {
	bytes.Buffer
	ReadDeadline time.Time
	Closed       bool
}

func newFakeConnection() *NyetConn {
	return &NyetConn{}
}

func (c *NyetConn) Close() error {
	c.Closed = true
	return nil
}

func (c *NyetConn) LocalAddr() net.Addr {
	return nil
}

func (c *NyetConn) RemoteAddr() net.Addr {
	return &net.IPAddr{
		IP:   net.ParseIP("1:2:3::a:b:c"),
		Zone: "",
	}
}

func (c *NyetConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *NyetConn) SetReadDeadline(t time.Time) error {
	c.ReadDeadline = t
	return nil
}

func (c *NyetConn) SetWriteDeadline(t time.Time) error {
	return nil
}
