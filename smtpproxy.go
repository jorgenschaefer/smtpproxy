package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jorgenschaefer/smtpproxy/errors"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

const SD_LISTEN_FDS_START int = 3

var relayHost string
var validRecipient *regexp.Regexp
var overrideRecipient string
var tlsConfig *tls.Config

type connState struct {
	Hostname     string
	Sender       string
	Recipients   []string
	QuitReceived bool
}

type commandArgs struct {
	Command string
	Args    string
	ErrArgs map[string]string
}
type handlerFunction func(*smtpd.Conn, *connState, *commandArgs) error

var commandMap map[string]handlerFunction = map[string]handlerFunction{
	"HELO":     handleHELO,
	"EHLO":     handleEHLO,
	"STARTTLS": handleSTARTTLS,
	"MAIL":     handleMAIL,
	"RCPT":     handleRCPT,
	"DATA":     handleDATA,
	"RSET":     handleRSET,
	"NOOP":     handleNOOP,
	"VRFY":     handleVRFY,
	"QUIT":     handleQUIT,
}

func main() {
	relayHost = os.Getenv("RELAY_HOST")
	if relayHost == "" {
		fmt.Println("No RELAY_HOST specified")
		os.Exit(1)
	}
	validRecipient = regexp.MustCompile(os.Getenv("VALID_RECIPIENTS"))
	overrideRecipient = os.Getenv("OVERRIDE_RECIPIENT")

	ln, err := Listen()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	tlsConfig, err = LoadTLSConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("SMTP proxy started; address=\"%s\"\n", ln.Addr())
	defer fmt.Printf("SMTP proxy stopped; address=\"%s\"\n", ln.Addr())

	for {
		c, err := ln.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go handleConnection(c)
	}
}

func Listen() (net.Listener, error) {
	listenPID := os.Getenv("LISTEN_PID")

	if listenPID == "" {
		address := os.Getenv("LISTEN_ADDRESS")

		if address == "" {
			address = ":25"
		}
		return net.Listen("tcp", address)
	} else {
		listenFDs := os.Getenv("LISTEN_FDS")

		pid, err := strconv.Atoi(listenPID)
		if err != nil {
			err = fmt.Errorf("Bad LISTEN_PID: %v", err)
			return nil, err
		}
		if os.Getpid() != pid {
			err = fmt.Errorf("Bad LISTEN_PID, expected %v got %v",
				os.Getpid(), pid)
			return nil, err
		}
		fdcount, err := strconv.Atoi(listenFDs)
		if err != nil {
			err = fmt.Errorf("Bad LISTEN_FDS: %v", err)
			return nil, err
		}
		if fdcount != 1 {
			err = fmt.Errorf("Bad LISTEN_FDS, expected 1, got %v",
				fdcount)
			return nil, err
		}
		f := os.NewFile(uintptr(SD_LISTEN_FDS_START), "LISTEN_FD")
		return net.FileListener(f)
	}
}

func handleConnection(netConn net.Conn) {
	defer netConn.Close()
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	srv := &connState{
		Hostname: hostname,
	}

	addr := netConn.RemoteAddr()
	fmt.Printf("New connection; client=\"%s\"\n", addr)
	defer fmt.Printf("Client disconnected; client=\"%s\"\n", addr)

	c := smtpd.NewConn(netConn)
	err = handleNewClient(c, srv, nil)
	if err != nil {
		fmt.Println("Error:", err.Error())
		if errors.DoFlytrap(err) {
			flyTrap(c)
		}
		return
	}
	for {
		command, args, err := c.ReadCommand("30s")
		if err != nil {
			fmt.Printf("Error reading from client; client=\"%s\" error=\"%s\"\n",
				c, err)
			return
		}
		err = handleCommand(c, srv, command, args)
		if err != nil {
			fmt.Println("Error:", err.Error())
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

func handleCommand(c *smtpd.Conn, srv *connState,
	command string, args string) error {

	cmd := &commandArgs{
		Command: command,
		Args:    args,
		ErrArgs: map[string]string{
			"client":  c.String(),
			"command": command,
		},
	}
	if c.IsTLS() {
		cmd.ErrArgs["protocol"] = "STARTTLS"
	} else {
		cmd.ErrArgs["protocol"] = "SMTP"
	}
	if args != "" {
		cmd.ErrArgs["args"] = args
	}

	if handler, ok := commandMap[command]; ok {
		return handler(c, srv, cmd)
	} else {
		return errors.Error("unknown command", cmd.ErrArgs)
	}
}

func handleNewClient(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	errArgs := map[string]string{
		"client": c.String(),
	}
	err := c.WriteLine(fmt.Sprintf("220-%s here, please hold",
		srv.Hostname))
	if err != nil {
		errArgs["error"] = err.Error()
		return errors.Error("writing server greeting", errArgs)
	}
	_, _, err = c.ReadCommand("5s")
	if err == nil {
		return errors.FlytrapError("client spoke before its turn", errArgs)
	}
	err = c.WriteLine("220 Thank you for holding, how may I help you?")
	if err != nil {
		return errors.Error("writing server greeting", errArgs)
	}
	return nil
}

func handleHELO(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	c.Reply(250, srv.Hostname)
	return nil
}

func handleEHLO(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	if tlsConfig == nil {
		c.Reply(250, srv.Hostname, "8BITMIME")
	} else {
		c.Reply(250, srv.Hostname, "8BITMIME", "STARTTLS")
	}
	return nil
}

func handleSTARTTLS(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	c.Reply(220, "Ready to start TLS")
	c.StartTLS(tlsConfig)
	return nil
}

func handleMAIL(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	if srv.Sender != "" {
		c.Reply(503, "Duplicate MAIL command")
		return errors.Error("duplicate MAIL command", cmd.ErrArgs)
	}
	sender, err := extractSender(cmd.Args)
	if err != nil {
		c.Reply(501, "Missing sender")
		return errors.FlytrapError("missing sender", cmd.ErrArgs)
	}
	srv.Sender = sender
	c.Reply(250, "Ok")
	return nil
}

func handleRCPT(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	if srv.Sender == "" {
		c.Reply(501, "No sender specified")
		return errors.Error("RCPT without MAIL", cmd.ErrArgs)
	}
	recipient, err := extractRecipient(cmd.Args)
	if err != nil {
		c.Reply(501, "Missing recipient")
		cmd.ErrArgs["error"] = err.Error()
		return errors.Error("missing recipient", cmd.ErrArgs)
	}
	if !validRecipient.MatchString(recipient) {
		c.Reply(550, "Relay access denied")
		return errors.Error("relay access denied", cmd.ErrArgs)
	}
	srv.Recipients = append(srv.Recipients, recipient)
	c.Reply(250, "Ok")
	return nil
}

func handleDATA(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	if srv.Sender == "" {
		c.Reply(503, "No sender specified")
		return errors.Error("DATA without MAIL", cmd.ErrArgs)
	}
	if len(srv.Recipients) == 0 {
		c.Reply(503, "No recipients specified")
		return errors.Error("DATA without RCPT", cmd.ErrArgs)
	}
	c.Reply(354, "End data with <CRLF>.<CRLF>")
	body, err := c.ReadDotBytes()
	if err != nil {
		c.Reply(501, "You confuse me")
		cmd.ErrArgs["error"] = err.Error()
		return errors.Error("error reading mail data", cmd.ErrArgs)
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
		fmt.Printf("Error delivering mail; client=\"%s\" sender=\"%s\" recipients=\"%s\" error=\"%s\"\n",
			c, srv.Sender, strings.Join(srv.Recipients, ", "),
			err)
		c.Reply(450, "Error delivering the mail, try again later")
		return nil
	}
	fmt.Printf("Mail sent; client=\"%s\" sender=\"%s\" recipients=\"%s\"\n",
		c, srv.Sender, strings.Join(srv.Recipients, ", "))
	c.Reply(250, "Ok")
	return nil
}

func handleRSET(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	srv.Sender = ""
	srv.Recipients = nil
	c.Reply(250, "Ok")
	return nil
}

func handleNOOP(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	c.Reply(250, "Ok")
	return nil
}

func handleVRFY(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	c.Reply(502, "Not implemented")
	return nil
}

func handleQUIT(c *smtpd.Conn, srv *connState, cmd *commandArgs) error {
	c.Reply(221, "Have a nice day")
	srv.QuitReceived = true
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

func LoadTLSConfig() (*tls.Config, error) {
	serverCertFile := os.Getenv("SERVER_CERT")
	serverKeyFile := os.Getenv("SERVER_KEY")
	if serverCertFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		err = fmt.Errorf("Error opening certificates: %v", err)
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return config, nil
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
