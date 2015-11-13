package errors

import "testing"

func TestError(t *testing.T) {
	err_map := map[string]string{
		"foo": "f",
		"bar": "b",
	}
	err := Error("the message", err_map)
	expected := "the message; bar=\"b\" foo=\"f\""
	if err.Error() != expected {
		t.Error("Expected ", expected, " got ", err.Error())
	}
}

func TestFlytrapError(t *testing.T) {
	var err error
	err = Error("the message", map[string]string{}).(CommandError).Flytrap()
	if !DoFlytrap(err) {
		t.Error("Expected to DoFlytrap, but did not")
	}
}
