package mwlib

import (
	"bufio"
	"io"
	"io/ioutil"
	"net/http"
	"os"
)

func ReadCookies() []*http.Cookie {
	cookieFile := GetWorkingDir() + "/cookies"
	file, err := os.Open(cookieFile)
	if err != nil {
		return nil
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	cookies := []*http.Cookie{}
	for i := 0; ; i++ {
		name, err := reader.ReadString(' ')
		if err == io.EOF {
			return cookies
		}
		if err != nil {
			panic(err)
		}
		name = name[:len(name)-1]
		value, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		value = value[:len(value)-1]
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}
}

func WriteCookies(cookies []*http.Cookie) {
	// Write to a temp file to avoid corruption if another instance writes simultaneously
	// (we don't care which one wins.)
	writer, err := ioutil.TempFile(GetWorkingDir(), "cookies")
	tmpFile := writer.Name()
	if err != nil {
		panic(err)
	}
	err = os.Chmod(tmpFile, 0600) // Protect session cookies.
	if err != nil {
		panic(err)
	}
	for i := range cookies {
		writer.WriteString(cookies[i].Name)
		writer.WriteString(" ")
		writer.WriteString(cookies[i].Value)
		writer.WriteString("\n")
	}
	writer.Close()
	cookieFile := GetWorkingDir() + "/cookies"
	err = os.Rename(tmpFile, cookieFile)
	if err != nil {
		panic(err)
	}

}
