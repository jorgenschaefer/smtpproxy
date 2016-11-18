// Package argerror implements an error type with key-value
// arguments. It's possible to build up an error step by step by
// adding more arguments.

package argerror

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type ArgError struct {
	message string
	args    map[string]string
}

func New(message string, args map[string]string) ArgError {
	return ArgError{message, args}
}

func (e ArgError) Error() string {
	var buffer bytes.Buffer

	keys := make([]string, 0, len(e.args))
	for k := range e.args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buffer.Write([]byte(e.message))
	if len(keys) > 0 {
		buffer.Write([]byte(";"))
	}
	for _, key := range keys {
		fmt.Fprintf(&buffer, " %s=\"%s\"", key, strings.Replace(e.args[key], "\n", " ", -1))
	}
	return buffer.String()
}
