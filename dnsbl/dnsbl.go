package dnsbl

import (
	"fmt"
	"net"
	"strings"
)

type LookupFunction func(string) ([]string, error)

type DNSBL struct {
	servers []string
	lookup  LookupFunction
}

func New(servers []string, lookup LookupFunction) *DNSBL {
	blacklist := &DNSBL{
		servers: make([]string, len(servers)),
		lookup:  lookup,
	}
	for i, srv := range servers {
		if strings.HasSuffix(srv, ".") {
			blacklist.servers[i] = srv
		} else {
			blacklist.servers[i] = srv + "."
		}

	}
	return blacklist
}

func (blacklist *DNSBL) Check(ipaddress net.Addr) (string, bool) {
	prefix := makePrefix(ipaddress)
	for _, srv := range blacklist.servers {
		hosts, err := blacklist.lookup(fmt.Sprintf("%s.%s", prefix, srv))
		if err == nil {
			return fmt.Sprintf("DNSBL %s returned %s", srv, hosts), true
		}
	}
	return "", false
}

func makePrefix(address net.Addr) string {
	tcpaddress, ok := address.(*net.TCPAddr)
	if !ok {
		panic("Check() did not receive a TCPAddr")
	}
	parsed := tcpaddress.IP

	if ip := parsed.To4(); ip != nil {
		reversed := make([]string, net.IPv4len)
		for i := 0; i < len(ip); i++ {
			val := int(ip[len(ip)-1-i])
			reversed[i] = fmt.Sprintf("%d", val)
		}
		return strings.Join(reversed, ".")
	} else if ip := parsed.To16(); ip != nil {
		reversed := make([]string, net.IPv6len*2)
		for i := 0; i < len(ip); i++ {
			val := int(ip[len(ip)-1-i])
			reversed[2*i] = fmt.Sprintf("%x", val&0x0F)
			reversed[2*i+1] = fmt.Sprintf("%x", val>>4)
		}
		return strings.Join(reversed, ".")
	} else {
		panic("Not an IPv4 nor IPv6 IP address")
	}
}
