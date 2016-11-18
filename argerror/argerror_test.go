package argerror

import "testing"

func TestNew(t *testing.T) {
	message := "Hello"
	args := map[string]string{
		"arg":  "Arg value",
		"barg": "Barg\nvalue\nbaz",
	}
	var err error = New(message, args)
	actual := err.Error()
	expected := "Hello; arg=\"Arg value\" barg=\"Barg value baz\""
	if actual != expected {
		t.Errorf("Expected error to be '%s', but was '%s'", expected, actual)
	}
}
