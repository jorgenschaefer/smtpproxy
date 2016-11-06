package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var validRecipients *regexp.Regexp

func Check() {
	if RelayHost() == "" {
		fmt.Fprintf(os.Stderr, "No RELAY_HOST given\n")
		os.Exit(1)
	}

	rx, err := regexp.Compile(os.Getenv("VALID_RECIPIENTS"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid regular expression VALID_RECIPIENTS: %v\n",
			err)
		os.Exit(1)
	}
	validRecipients = rx
	// Listening stuff
	listenpid := os.Getenv("LISTEN_PID")
	if listenpid != "" {
		wantedpid, err := strconv.Atoi(listenpid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LISTEN_PID is not an integer: %v\n", err)
			os.Exit(1)
		}
		actualpid := os.Getpid()
		if wantedpid != actualpid {
			fmt.Fprintf(os.Stderr, "LISTEN_PID is for process %d, we are %d\n",
				wantedpid, actualpid)
			os.Exit(1)
		}
		listenfds := os.Getenv("LISTEN_FDS")
		fdcount, err := strconv.Atoi(listenfds)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LISTEN_FDS is not an integer: %v\n", err)
			os.Exit(1)
		}
		if fdcount != 1 {
			fmt.Fprintf(os.Stderr, "Got %d listening sockets, expected one\n",
				fdcount)
			os.Exit(1)
		}
	}
}

func DNSBL() []string {
	return strings.Split(os.Getenv("DNSBL_DOMAINS"), " ")
}

func ValidRecipient() *regexp.Regexp {
	return validRecipients
}

func OverrideRecipient() (string, bool) {
	override := os.Getenv("OVERRIDE_RECIPIENT")
	if override != "" {
		return override, true
	} else {
		return "", false
	}
}

func RelayHost() string {
	return os.Getenv("RELAY_HOST")
}

func ListenMode() string {
	if os.Getenv("LISTEN_PID") == "" {
		return "address"
	} else {
		return "systemd"
	}
}

func ListenAddress() string {
	address := os.Getenv("LISTEN_ADDRESS")

	if address == "" {
		return ":25"
	} else {
		return address
	}
}

const SD_LISTEN_FDS_START uintptr = 3

func ListenFD() uintptr {
	return SD_LISTEN_FDS_START
}

func TLS() (*tls.Config, bool) {
	return nil, false
}
