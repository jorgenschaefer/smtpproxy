package main

import (
	"bytes"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"testing"

	"github.com/jorgenschaefer/smtpproxy/config"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

func TestSMTPProxy(t *testing.T) {
	// Start relay
	smtpln, err := net.Listen("tcp", "")
	if err != nil {
		t.Error(err)
	}
	var buf bytes.Buffer
	go readMail(smtpln, &buf)
	os.Setenv("RELAY_HOST", smtpln.Addr().String())
	config.Check()
	proxyln, err := net.Listen("tcp", "")
	if err != nil {
		t.Error(err)
	}
	// Start proxy server
	go func() {
		conn, err := proxyln.Accept()
		if err != nil {
			panic(err)
		}
		handleConnection(smtpd.NewConnection(conn))
	}()
	// Send mail to the proxy server
	err = smtp.SendMail(proxyln.Addr().String(), nil, "me@test.tld",
		[]string{"you@test.tld"}, []byte("Hello"))
	if err != nil {
		t.Error(err)
	}
	data := buf.String()
	expected := "EHLO localhost\r\nMAIL FROM:<me@test.tld>\r\nRCPT TO:<you@test.tld>\r\nDATA\r\nHello\r\n.\r\nQUIT\r\n"
	if data != expected {
		t.Errorf("Expected a mail, got %#v", data)
	}
}

func readMail(ln net.Listener, buf *bytes.Buffer) {
	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "220 Hi\r\n250 Ok\r\n250 Ok\r\n250 Ok\r\n354 Ok\r\n250 Ok\r\n221 Ok\r\n")
	b := make([]byte, 4096)
	for {
		n, err := conn.Read(b)
		buf.Write(b[:n])
		if err != nil {
			return
		}
	}
}
