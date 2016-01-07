package main

import (
	"errors"
	"fmt"
	"strconv"
)

// A timestamp is either a string representation of a date/time or
// blank and not valid if not specified.
type timestamp struct {
	string string
	valid  bool
}

func newTimestamp(input string, valid bool) (timestamp, error) {
	if !valid {
		return timestamp{"", false}, nil
	}
	// checking is basic, the API will reject invalid values anyway.
	if len(input) != 14 {
		return timestamp{"", false}, errors.New("invalid timestamp")
	}
	_, err := strconv.Atoi(input)
	if err != nil {
		return timestamp{"", false}, errors.New("invalid timestamp")
	}
	return timestamp{input, true}, nil
}

func printBadTimestamp() {
	fmt.Println("Invalid timestamp. Format is YYYYMMDDHHMMSS.")
}
