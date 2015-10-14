package mwlib

import (
	"bufio"
	"io"
        "net/http"
	"os"
)

func ReadCookies() ([]*http.Cookie) {
	cookieFile := GetWorkingDir() + "/cookies"
	file, err := os.Open(cookieFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	cookies := []*http.Cookie{}
	for i := 0;; i++ {
		name, err := reader.ReadString(' ')
		if (err == io.EOF) {
			return cookies;
		} else if err != nil {
			panic(err)
		}
		name = name[:len(name) - 1]
		value, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		value = value[:len(value) - 1]
		cookies = append(cookies, &http.Cookie{Name:name, Value:value})
	}
	return cookies
}
