package dnsbl

import (
	"fmt"
	"net"
	"strings"
)

func Domain(ipaddress, dnsbl string) (string, error) {
	parsed := net.ParseIP(ipaddress)

	if ip := parsed.To4(); ip != nil {
		reversed := make([]string, net.IPv4len)
		for i := 0; i < len(ip); i++ {
			val := int(ip[len(ip)-1-i])
			reversed[i] = fmt.Sprintf("%d", val)
		}
		prefix := strings.Join(reversed, ".")
		return fmt.Sprintf("%s.%s", prefix, dnsbl), nil
	} else if ip := parsed.To16(); ip != nil {
		reversed := make([]string, net.IPv6len*2)
		for i := 0; i < len(ip); i++ {
			val := int(ip[len(ip)-1-i])
			reversed[2*i] = fmt.Sprintf("%x", val&0x0F)
			reversed[2*i+1] = fmt.Sprintf("%x", val>>4)
		}
		prefix := strings.Join(reversed, ".")
		return fmt.Sprintf("%s.%s", prefix, dnsbl), nil
	} else {
		return "", fmt.Errorf("Invalid IP address: %v", ip)
	}
}

func Lookup(ipaddress string, dnsblList []string) (string, error) {
	for _, baseDomain := range dnsblList {
		domain, err := Domain(ipaddress, baseDomain)
		if err != nil {
			return "", err
		}
		host, err := net.LookupHost(domain)
		if err == nil {
			reason := fmt.Sprintf("dnsbl %s returned %s",
				baseDomain, host)
			return reason, nil
		}
	}
	return "", nil
}
