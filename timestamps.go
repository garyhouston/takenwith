package main

import (
	"errors"
	"strconv"
)

// A timestamp is either a string representation of a date/time or
// blank and not valid if not specified.
type timestamp struct {
	string string
	valid  bool
}

func newTimestampEmpty() timestamp {
	return timestamp{"", false}
}

func newTimestamp(input string) (timestamp, error) {
	makeErr := func() error {
		return errors.New("Invalid timestamp. Format must be YYYYMMDDHHMMSS.")
	}
	// Basic checks, the MediaWiki API will reject invalid values anyway.
	if len(input) != 14 {
		return newTimestampEmpty(), makeErr()
	}
	_, err := strconv.Atoi(input)
	if err != nil {
		return newTimestampEmpty(), makeErr()
	}
	return timestamp{input, true}, nil
}
