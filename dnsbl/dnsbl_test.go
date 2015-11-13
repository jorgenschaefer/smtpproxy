package dnsbl

import "testing"

func TestDomain(t *testing.T) {
	actual, err := Domain("10.11.12.13", "rbl.tld")
	if err != nil {
		t.Error("Unexpected error", err)
	}
	expected := "13.12.11.10.rbl.tld"
	if actual != expected {
		t.Error("Expected", expected, "got", actual)
	}

	actual, err = Domain("2001:DB8:abc:123::42", "rbl.tld")
	if err != nil {
		t.Error("Unexpected error", err)
	}
	expected = "2.4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.2.1.0.c.b.a.0.8.b.d.0.1.0.0.2.rbl.tld"
	if actual != expected {
		t.Error("Expected", expected, "got", actual)
	}
}
