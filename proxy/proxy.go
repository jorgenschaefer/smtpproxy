package proxy

import (
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"regexp"
	"strings"

	"github.com/jorgenschaefer/smtpproxy/argerror"
	"github.com/jorgenschaefer/smtpproxy/config"
	"github.com/jorgenschaefer/smtpproxy/dnsbl"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

type State struct {
	conn       smtpd.Connection
	sender     string
	recipients []string
	args       map[string]string
	blacklist  *dnsbl.DNSBL
}

func Greet(conn smtpd.Connection) (*State, error) {
	s := &State{
		conn:      conn,
		args:      map[string]string{},
		blacklist: dnsbl.New(config.DNSBL(), net.LookupHost),
	}
	s.args["client"] = s.conn.RemoteAddr().String()
	if err := conn.Printf("220-%s here, please hold.\r\n", hostname()); err != nil {
		s.args["error"] = err.Error()
		return nil, s.Error("Error writing server greeting")
	}
	command, args, err := conn.ReadCommand(5)
	if err == nil {
		s.args["command"] = command
		if args != "" {
			s.args["command"] += " " + args
		}
		return nil, s.TarpitError("Error: Client spoke before its turn")
	}
	if neterr, ok := err.(net.Error); !ok || !neterr.Timeout() {
		s.args["error"] = err.Error()
		return nil, s.Error("Error during greeting")
	}

	if err := conn.Reply(220, "Thank you for holding, how can I help you?"); err != nil {
		s.args["error"] = err.Error()
		return nil, s.Error("Error writing server greeting continuation")
	}
	return s, nil
}

func (s *State) HandleCommand() error {
	command, args, err := s.conn.ReadCommand(30)
	if err != nil {
		s.args["error"] = err.Error()
		return s.Error("Error reading client command")
	}
	s.args["command"] = command
	if args != "" {
		s.args["command"] += " " + args
	}
	switch strings.ToUpper(command) {
	case "HELO":
		s.conn.Reply(250, hostname())
		s.args["protocol"] = "SMTP"
	case "EHLO":
		if _, ok := config.TLS(); ok {
			s.conn.Reply(250, hostname(), "8BITMIME", "STARTTLS")
		} else {
			s.conn.Reply(250, hostname(), "8BITMIME")
		}
		s.args["protocol"] = "ESMTP"
	case "STARTTLS":
		if tls, ok := config.TLS(); ok {
			s.conn.Reply(220, "Ready to start TLS")
			s.conn.StartTLS(tls)
			s.args["protocol"] = "ESMTPS"
		} else {
			return s.Error("Error: Unexpected STARTTLS command")
		}
	case "MAIL":
		return s.handleMail(args)
	case "RCPT":
		return s.handleRcpt(args)
	case "DATA":
		return s.HandleData()
	case "RSET":
		s.Reset()
		s.conn.Reply(250, "Ok")
	case "NOOP":
		s.conn.Reply(250, "Ok")
	case "VRFY":
		s.conn.Reply(502, "Not implemented")
	case "QUIT":
		s.conn.Reply(221, "Have a nice day")
		return s.Error("Client QUIT")
	default:
		return s.TarpitError("Error: Unknown command")
	}
	return nil
}

var PERMANENTARGS = []string{"client", "protocol"}

func (s *State) Reset() {
	s.sender = ""
	s.recipients = []string{}
	args := map[string]string{}
	for _, key := range PERMANENTARGS {
		if val, ok := s.args[key]; ok {
			args[key] = val
		}
	}
	s.args = args
}

func (s *State) Error(description string) error {
	return argerror.New(description, s.args)
}

type TarpitError struct {
	argerror.ArgError
}

func (s *State) TarpitError(description string) error {
	return TarpitError{s.Error(description).(argerror.ArgError)}
}

func (s *State) handleMail(args string) error {
	if s.sender != "" {
		return s.TarpitError("Error: Duplicate MAIL command")
	}
	sender, ok := extractSender(args)
	if !ok {
		return s.TarpitError("Error: Syntax error in MAIL command")
	}
	s.sender = sender
	s.args["sender"] = sender
	s.conn.Reply(250, "Ok")
	return nil
}

func (s *State) handleRcpt(args string) error {
	if s.sender == "" {
		return s.TarpitError("Error: RCPT without MAIL")
	}
	recipient, ok := extractRecipient(args)
	if !ok {
		return s.TarpitError("Error: Syntax error in RCPT command")
	}
	if !isValidRecipient(recipient) {
		s.args["recipient"] = recipient
		return s.TarpitError("Error: Relay access denied")
	}
	s.recipients = append(s.recipients, recipient)
	s.args["recipients"] = strings.Join(s.recipients, ", ")
	s.conn.Reply(250, "Ok")
	return nil

}

func (s *State) HandleData() error {
	if s.sender == "" {
		return s.TarpitError("Error: DATA without MAIL")
	}
	if len(s.recipients) == 0 {
		return s.TarpitError("Error: DATA without RCPT")
	}
	s.conn.Reply(354, "End data with <CRLF>.<CRLF>")
	body, err := s.conn.ReadDotBytes(5 * 60)
	if err != nil {
		s.conn.Reply(501, "You confuse me")
		s.args["error"] = err.Error()
		return s.Error("Error reading mail data")
	}
	recipients := s.recipients
	if override, ok := config.OverrideRecipient(); ok {
		recipients = []string{override}
	}
	if msg, ok := s.blacklist.Check(s.conn.RemoteAddr()); ok {
		s.args["dnsbl"] = msg
		return s.TarpitError("Error: DNSBL check positive")
	}
	if err := smtp.SendMail(config.RelayHost(), nil, s.sender, recipients, body); err != nil {
		s.args["error"] = err.Error()
		if protoErr, ok := err.(*textproto.Error); ok {
			s.conn.Reply(protoErr.Code, "Error delivering the mail")
			return s.TarpitError("Error delivering mail")
		} else {
			s.conn.Reply(450, "Error delivering the mail, try again later")
			return s.Error("Error delivering mail")
		}
	}
	fmt.Println(s.Error("Mail sent"))
	s.conn.Reply(250, "Ok")
	s.Reset()
	return nil
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return name
}

var mailFrom = regexp.MustCompile("(?i)from:<(.+)>")

func extractSender(data string) (string, bool) {
	found := mailFrom.FindStringSubmatch(data)
	if found == nil {
		return "", false
	}
	return found[1], true
}

var rcptTo = regexp.MustCompile("(?i)to:<(.+)>")

func extractRecipient(data string) (string, bool) {
	found := rcptTo.FindStringSubmatch(data)
	if found == nil {
		return "", false
	}
	return found[1], true
}

func isValidRecipient(recipient string) bool {
	return config.ValidRecipient().MatchString(recipient)
}
