package errors

import (
	"bytes"
	"fmt"
	"sort"
)

type CommandError struct {
	message   string
	doFlytrap bool
}

func (c CommandError) Error() string {
	return c.message
}

func DoFlytrap(err error) bool {
	if cerr, ok := err.(CommandError); ok {
		return cerr.doFlytrap
	}
	return false
}

func Error(message string, err_args map[string]string) error {
	var buffer bytes.Buffer

	var keys []string
	for key := range err_args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buffer.Write([]byte(message))
	if len(keys) > 0 {
		buffer.Write([]byte(";"))
	}
	for _, key := range keys {
		arg := fmt.Sprintf(" %s=\"%s\"", key, err_args[key])
		buffer.Write([]byte(arg))
	}

	return CommandError{
		message: buffer.String(),
	}
}

func FlytrapError(message string, err_args map[string]string) error {
	err := Error(message, err_args)
	if cerr, ok := err.(CommandError); ok {
		cerr.doFlytrap = true
		return cerr
	}
	return err
}
