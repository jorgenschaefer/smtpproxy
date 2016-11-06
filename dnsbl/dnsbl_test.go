package dnsbl

import (
	"errors"
	"net"
	"testing"
)

func TestCheck(t *testing.T) {
	args := []string{}
	result := []string{}
	blacklist := New([]string{"rbl.tld", "rbl2.tld."},
		func(host string) ([]string, error) {
			args = append(args, host)
			if len(result) == 0 {
				return result, errors.New("Failed to resolve")
			} else {
				return result, nil
			}
		})
	blacklist.Check(makeAddr("1.2.3.4"))
	if len(args) != 2 || args[0] != "4.3.2.1.rbl.tld." || args[1] != "4.3.2.1.rbl2.tld." {
		t.Errorf("Unexpected check order: %#v", args)
	}

	_, ok := blacklist.Check(makeAddr("1.2.3.4"))
	if ok {
		t.Error("Did not expect a positive result")
	}

	result = []string{"127.0.0.10"}
	description, ok := blacklist.Check(makeAddr("1.2.3.4"))
	if !ok {
		t.Error("Did expect a positive result")
	}
	expected := "DNSBL rbl.tld. returned [127.0.0.10]"
	if description != expected {
		t.Errorf("Expected %#v to equal %#v", description, expected)
	}
}

func TestMakePrefix(t *testing.T) {
	tests := map[string]string{
		"10.11.12.13":          "13.12.11.10",
		"2001:DB8:abc:123::42": "2.4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.2.1.0.c.b.a.0.8.b.d.0.1.0.0.2",
	}
	for address, prefix := range tests {
		actual := makePrefix(makeAddr(address))
		if actual != prefix {
			t.Errorf("Expected %#v to turn into %#v, but got %#v",
				address, prefix, actual)
		}
	}
}

func makeAddr(address string) net.Addr {
	return &net.TCPAddr{
		IP: net.ParseIP(address),
	}
}
