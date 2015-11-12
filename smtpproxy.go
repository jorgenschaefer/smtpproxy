package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"regexp"
	"strings"

	"github.com/jorgenschaefer/smtpproxy/errors"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

var serverCertFile string
var serverKeyFile string
var validRecipient *regexp.Regexp
var relayHost string
var overrideRecipient string

func main() {
	serverCertFile = os.Getenv("SERVER_CERT")
	serverKeyFile = os.Getenv("SERVER_KEY")
	validRecipient = regexp.MustCompile(os.Getenv("VALID_RECIPIENTS"))
	relayHost = os.Getenv("RELAY_HOST")
	if relayHost == "" {
		log.Fatal("No RELAY_HOST specified")
	}
	overrideRecipient = os.Getenv("OVERRIDE_RECIPIENT")

	address := os.Getenv("LISTEN_ADDRESS")
	if address == "" {
		address = ":25"
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("SMTP proxy started; address=\"%s\"", ln.Addr())

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		go handleConnection(c)
	}
}

type smtpState struct {
	Hostname     string
	Sender       string
	Recipients   []string
	IsTLS        bool
	QuitReceived bool
}

func handleConnection(netConn net.Conn) {
	defer netConn.Close()
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	addr := netConn.RemoteAddr()
	log.Printf("New connection; client=\"%s\"", addr)
	defer log.Printf("Client disconnected; client=\"%s\"", addr)
	c := smtpd.NewConn(netConn)
	err = greet(c, hostname)
	if err != nil {
		log.Print("Error: ", err.Error())
		if errors.DoFlytrap(err) {
			flyTrap(c)
		}
		return
	}
	srv := &smtpState{
		Hostname: hostname,
	}
	for {
		command, args, err := c.ReadCommand("30s")
		if err != nil {
			log.Printf("Error reading from client; client=\"%s\" error=\"%s\"",
				c, err)
			return
		}
		err = handleCommand(c, srv, command, args)
		if err != nil {
			log.Print("Error: ", err.Error())
			if errors.DoFlytrap(err) {
				flyTrap(c)
				return
			}
		}
		if srv.QuitReceived {
			break
		}
	}
}

func greet(c *smtpd.Conn, hostname string) error {
	err_args := map[string]string{
		"client": c.String(),
	}
	err := c.WriteLine(fmt.Sprintf("220-%s here, please hold", hostname))
	if err != nil {
		err_args["error"] = err.Error()
		return errors.Error("writing server greeting", err_args)
	}
	_, _, err = c.ReadCommand("5s")
	if err == nil {
		return errors.FlytrapError("client spoke before its turn", err_args)
	}
	err = c.WriteLine("220 Thank you for holding, how may I help you?")
	if err != nil {
		return errors.Error("writing server greeting", err_args)
	}
	return nil
}

func handleCommand(c *smtpd.Conn, srv *smtpState,
	command string, args string) error {

	err_args := map[string]string{
		"client":  c.String(),
		"command": command,
	}
	if srv.IsTLS {
		err_args["protocol"] = "STARTTLS"
	} else {
		err_args["protocol"] = "SMTP"
	}
	if args != "" {
		err_args["args"] = args
	}

	switch command {
	case "HELO":
		c.Reply(250, srv.Hostname)
	case "EHLO":
		if serverCertFile == "" {
			c.Reply(250, srv.Hostname, "8BITMIME")
		} else {
			c.Reply(250, srv.Hostname, "8BITMIME", "STARTTLS")
		}
	case "STARTTLS":
		c.Reply(220, "Ready to start TLS")
		c.StartTLS(tlsServerConf())
		srv.IsTLS = true
	case "MAIL":
		if srv.Sender != "" {
			c.Reply(503, "Duplicate MAIL command")
			return errors.Error("duplicate MAIL command", err_args)
		}
		sender, err := extractSender(args)
		if err != nil {
			c.Reply(501, "Missing sender")
			return errors.FlytrapError("missing sender", err_args)
		}
		srv.Sender = sender
		c.Reply(250, "Ok")
	case "RCPT":
		if srv.Sender == "" {
			c.Reply(501, "No sender specified")
			return errors.Error("RCPT without MAIL", err_args)
		}
		recipient, err := extractRecipient(args)
		if err != nil {
			c.Reply(501, "Missing recipient")
			err_args["error"] = err.Error()
			return errors.Error("missing recipient", err_args)
		}
		if !validRecipient.MatchString(recipient) {
			c.Reply(550, "Relay access denied")
			return errors.Error("relay access denied", err_args)
		}
		srv.Recipients = append(srv.Recipients, recipient)
		c.Reply(250, "Ok")
	case "DATA":
		if srv.Sender == "" {
			c.Reply(503, "No sender specified")
			return errors.Error("DATA without MAIL", err_args)
		}
		if len(srv.Recipients) == 0 {
			c.Reply(503, "No recipients specified")
			return errors.Error("DATA without RCPT", err_args)
		}
		c.Reply(354, "End data with <CRLF>.<CRLF>")
		body, err := c.ReadDotBytes()
		if err != nil {
			c.Reply(501, "You confuse me")
			err_args["error"] = err.Error()
			return errors.Error("error reading mail data", err_args)
		}
		recipients := srv.Recipients
		if overrideRecipient != "" {
			recipients = []string{overrideRecipient}
		}
		err = smtp.SendMail(
			relayHost,
			nil,
			srv.Sender,
			recipients,
			body,
		)
		if err != nil {
			log.Printf("Error delivering mail; client=\"%s\" sender=\"%s\" recipients=\"%s\" error=\"%s\"",
				c, srv.Sender, strings.Join(srv.Recipients, ", "),
				err)
			c.Reply(450, "Error delivering the mail, try again later")
			return nil
		}
		log.Printf("Mail sent; client=\"%s\" sender=\"%s\" recipients=\"%s\"",
			c, srv.Sender, strings.Join(srv.Recipients, ", "))
		c.Reply(250, "Ok")
	case "RSET":
		srv.Sender = ""
		srv.Recipients = nil
		c.Reply(250, "Ok")
	case "NOOP":
		c.Reply(250, "Ok")
	case "VRFY":
		c.Reply(502, "Not implemented")
	case "QUIT":
		c.Reply(221, "Have a nice day")
		srv.QuitReceived = true
		return nil
	default:
		return errors.Error("unknown command", err_args)
	}
	return nil
}

func flyTrap(c *smtpd.Conn) {
	for {
		_, err := c.ReadLine("300s")
		if err != nil {
			return
		}
	}
}

func tlsServerConf() *tls.Config {
	cert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		log.Fatal("Error opening certificates: ", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return config
}

var mailFrom = regexp.MustCompile("(?i)from:<(.+)>")

func extractSender(data string) (string, error) {
	found := mailFrom.FindStringSubmatch(data)
	if found == nil {
		return "", fmt.Errorf("invalid sender specification")
	}
	return found[1], nil
}

var rcptTo = regexp.MustCompile("(?i)to:<(.+)>")

func extractRecipient(data string) (string, error) {
	found := rcptTo.FindStringSubmatch(data)
	if found == nil {
		return "", fmt.Errorf("invalid recipient specification")
	}
	return found[1], nil
}
