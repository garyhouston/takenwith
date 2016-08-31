package mwlib

import (
	"bufio"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

func ReadCookies(cookieFile string) []*http.Cookie {
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

func WriteCookies(cookies []*http.Cookie, cookieFile string) {
	// Write to a temp file to avoid corruption if another instance writes simultaneously
	// (we don't care which one wins.)
	dir, file := filepath.Split(cookieFile)
	writer, err := ioutil.TempFile(dir, file)
	if err != nil {
		panic(err)
	}
	tmpFile := writer.Name()
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
	err = os.Rename(tmpFile, cookieFile)
	if err != nil {
		panic(err)
	}

}
