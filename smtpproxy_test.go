package main

import "testing"

func TestExtractSender(t *testing.T) {
	var goodCases = map[string]string{
		"from:<foo@bar.com>": "foo@bar.com",
		"FROM:<foo@bar.com>": "foo@bar.com",
		"FrOm:<foo@bar.com>": "foo@bar.com",
	}
	for k, v := range goodCases {
		sender, err := extractSender(k)
		if err != nil {
			t.Error("When parsing", k, "expected success, got",
				err)
		}
		if sender != v {
			t.Error("When parsing", k, "expected", v,
				", got", sender)
		}
	}

	var badCases = []string{
		"from:",
	}
	for _, v := range badCases {
		sender, err := extractSender(v)
		if err == nil {
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
		sender, err := extractRecipient(k)
		if err != nil {
			t.Error("When parsing", k, "expected success, got",
				err)
		}
		if sender != v {
			t.Error("When parsing", k, "expected", v,
				", got", sender)
		}
	}

	var badCases = []string{
		"to:",
	}
	for _, v := range badCases {
		sender, err := extractRecipient(v)
		if err == nil {
			t.Error("Expected failure, got ", sender)
		}
	}
}
