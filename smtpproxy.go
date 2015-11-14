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

	"github.com/jorgenschaefer/smtpproxy/dnsbl"
	"github.com/jorgenschaefer/smtpproxy/errors"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

const SD_LISTEN_FDS_START int = 3

var relayHost string
var validRecipient *regexp.Regexp
var overrideRecipient string
var tlsConfig *tls.Config

type handlerFunction func(*smtpd.Conn, *connState) error

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

type connState struct {
	Client       *smtpd.Conn
	Hostname     string
	Protocol     string
	Sender       string
	Recipients   []string
	QuitReceived bool
	Command      string
	Args         string
	Err          error
	DNSBL        string
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

	addr := netConn.RemoteAddr()
	fmt.Printf("New connection; client=\"%s\"\n", addr)
	defer fmt.Printf("Client disconnected; client=\"%s\"\n", addr)

	c := smtpd.NewConn(netConn)

	srv := &connState{
		Hostname: hostname,
		Client:   c,
		Protocol: "SMTP",
	}

	err = handleNewClient(c, srv)
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
		srv.Command = command
		srv.Args = args
		handler, ok := commandMap[command]
		if !ok {
			fmt.Println(srv.Error("unknown command"))
			flyTrap(c)
			return
		}
		err = handler(c, srv)
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

func handleNewClient(c *smtpd.Conn, srv *connState) error {
	err := c.WriteLine(fmt.Sprintf("220-%s here, please hold",
		srv.Hostname))
	if err != nil {
		srv.Err = err
		return srv.Error("writing server greeting")
	}
	_, _, err = c.ReadCommand("5s")
	if err == nil {
		return srv.Error("client spoke before its turn").Flytrap()
	}
	err = c.WriteLine("220 Thank you for holding, how may I help you?")
	if err != nil {
		return srv.Error("writing server greeting")
	}
	return nil
}

func handleHELO(c *smtpd.Conn, srv *connState) error {
	c.Reply(250, srv.Hostname)
	return nil
}

func handleEHLO(c *smtpd.Conn, srv *connState) error {
	if tlsConfig == nil {
		c.Reply(250, srv.Hostname, "8BITMIME")
	} else {
		c.Reply(250, srv.Hostname, "8BITMIME", "STARTTLS")
	}
	if srv.Protocol == "SMTP" {
		srv.Protocol = "ESMTP"
	}
	return nil
}

func handleSTARTTLS(c *smtpd.Conn, srv *connState) error {
	c.Reply(220, "Ready to start TLS")
	c.StartTLS(tlsConfig)
	srv.Protocol = "ESMTPS"
	return nil
}

func handleMAIL(c *smtpd.Conn, srv *connState) error {
	if srv.Sender != "" {
		c.Reply(503, "Duplicate MAIL command")
		return srv.Error("duplicate MAIL command")
	}
	sender, err := extractSender(srv.Args)
	if err != nil {
		c.Reply(501, "Missing sender")
		srv.Err = err
		return srv.Error("missing sender").Flytrap()
	}
	srv.Sender = sender
	c.Reply(250, "Ok")
	return nil
}

func handleRCPT(c *smtpd.Conn, srv *connState) error {
	if srv.Sender == "" {
		c.Reply(501, "No sender specified")
		return srv.Error("RCPT without MAIL")
	}
	recipient, err := extractRecipient(srv.Args)
	if err != nil {
		c.Reply(501, "Missing recipient")
		srv.Err = err
		return srv.Error("missing recipient")
	}
	if !validRecipient.MatchString(recipient) {
		c.Reply(550, "Relay access denied")
		return srv.Error("relay access denied")
	}
	srv.Recipients = append(srv.Recipients, recipient)
	c.Reply(250, "Ok")
	return nil
}

func handleDATA(c *smtpd.Conn, srv *connState) error {
	if srv.Sender == "" {
		c.Reply(503, "No sender specified")
		return srv.Error("DATA without MAIL")
	}
	if len(srv.Recipients) == 0 {
		c.Reply(503, "No recipients specified")
		return srv.Error("DATA without RCPT")
	}
	c.Reply(354, "End data with <CRLF>.<CRLF>")
	body, err := c.ReadDotBytes()
	if err != nil {
		c.Reply(501, "You confuse me")
		srv.Err = err
		return srv.Error("error reading mail data")
	}
	recipients := srv.Recipients
	if overrideRecipient != "" {
		recipients = []string{overrideRecipient}
	}
	if reason := DNSBL(c.String()); reason != "" {
		srv.DNSBL = reason
		return srv.Error("DNSBL check positive")
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
	fmt.Printf("Mail sent; client=\"%s\" recipients=\"%s\" sender=\"%s\" protocol=\"%s\"\n",
		c, strings.Join(srv.Recipients, ", "), srv.Sender, srv.Protocol)
	c.Reply(250, "Ok")
	return nil
}

func handleRSET(c *smtpd.Conn, srv *connState) error {
	srv.Sender = ""
	srv.Recipients = nil
	c.Reply(250, "Ok")
	return nil
}

func handleNOOP(c *smtpd.Conn, srv *connState) error {
	c.Reply(250, "Ok")
	return nil
}

func handleVRFY(c *smtpd.Conn, srv *connState) error {
	c.Reply(502, "Not implemented")
	return nil
}

func handleQUIT(c *smtpd.Conn, srv *connState) error {
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

func (srv *connState) Error(message string) errors.CommandError {
	args := map[string]string{
		"client": srv.Client.String(),
	}
	if srv.Sender != "" {
		args["sender"] = srv.Sender
	}
	if len(srv.Recipients) > 0 {
		args["recipients"] = strings.Join(srv.Recipients, ", ")
	}
	if srv.Protocol != "" {
		args["protocol"] = srv.Protocol
	}
	if srv.Command != "" {
		args["command"] = srv.Command
	}
	if srv.Args != "" {
		args["args"] = srv.Args
	}
	if srv.Err != nil {
		args["error"] = srv.Err.Error()
	}
	if srv.DNSBL != "" {
		args["dnsbl"] = srv.DNSBL
	}
	return errors.Error(message, args).(errors.CommandError)
}

func DNSBL(ipaddress string) string {
	domainstring := os.Getenv("DNSBL_DOMAINS")
	if domainstring == "" {
		return ""
	}
	dnsblList := strings.Split(domainstring, " ")
	for i := range dnsblList {
		dnsblList[i] = strings.Trim(dnsblList[i], ", ")
	}

	host, _, err := net.SplitHostPort(ipaddress)
	if err != nil {
		fmt.Printf("[dnsbl] Error in net.SplitHostString(\"%s\"): %v",
			ipaddress, err)
		return ""
	}

	reason, err := dnsbl.Lookup(host, dnsblList)
	if err != nil {
		fmt.Printf("[dnsbl] Error in dnsbl.Lookup(%v): %v\n",
			host, err)
		return ""
	}
	return reason
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
