package proxy

import "testing"

func TestExtractSender(t *testing.T) {
	var goodCases = map[string]string{
		"from:<foo@bar.com>":          "foo@bar.com",
		"FROM:<foo@bar.com>":          "foo@bar.com",
		"FrOm:<foo@bar.com>":          "foo@bar.com",
		"from:<foo@bar.com> 8BITMIME": "foo@bar.com",
	}
	for k, v := range goodCases {
		sender, ok := extractSender(k)
		if !ok {
			t.Errorf("Failed parsing %#v", k)
		}
		if sender != v {
			t.Errorf("Parsed %#v as %#v, expected %#v",
				k, sender, v)
		}
	}

	var badCases = []string{
		"from:",
	}
	for _, v := range badCases {
		sender, ok := extractSender(v)
		if ok {
			t.Error("Expected failure, got ", sender)
		}
	}
}

func TestExtractRecipient(t *testing.T) {
	var goodCases = map[string]string{
		"to:<foo@bar.com>": "foo@bar.com",
		"TO:<foo@bar.com>": "foo@bar.com",
		"To:<foo@bar.com>": "foo@bar.com",
	}
	for k, v := range goodCases {
		sender, ok := extractRecipient(k)
		if !ok {
			t.Errorf("Failed parsing %#v", sender)
		}
		if sender != v {
			t.Errorf("Parsed %#v as %#v, expected %#v",
				k, sender, v)
		}
	}

	var badCases = []string{
		"to:",
	}
	for _, v := range badCases {
		sender, ok := extractRecipient(v)
		if ok {
			t.Errorf("Expected failure, got %#v", sender)
		}
	}
}
