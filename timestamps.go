package main

import (
	"errors"
	"fmt"
	"strconv"
)

type timestamp string

func newTimestamp(input string) (timestamp, error) {
	// checking is basic, the API will reject invalid values anyway.
	if len(input) != 14 {
		return "", errors.New("invalid timestamp")
	}
	_, err := strconv.Atoi(input)
	if err != nil {
		return "", errors.New("invalid timestamp")
	}
	return timestamp(input), nil
}

// some arbitrary future value that is accepted by the API.
func futureTimestamp() timestamp {
	ts, err := newTimestamp("20990101000000")
	if err != nil {
		panic(err)
	}
	return ts
}

func printBadTimestamp() {
	fmt.Println("Invalid timestamp. Format is YYYYMMDDHHMMSS.")
}
