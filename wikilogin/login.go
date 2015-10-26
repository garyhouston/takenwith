package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"github.com/garyhouston/takenwith/mwlib"
	"log"
	"os"
)

// This login program must be run before using the main bot. It saves
// cookies into a file in the bot's directory.
func main() {
	if len(os.Args) != 4 {
		log.Fatal("Usage: ", os.Args[0], " username password operator@email")
	}
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "wikilogin "+os.Args[3])
	if err != nil {
		panic(err)
	}
	client.Maxlag.On = true

	err = client.Login(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	cookies := client.DumpCookies()
	cookieFile := mwlib.GetWorkingDir() + "/cookies"
	writer, err := os.Create(cookieFile)
	if err != nil {
		panic(err)
	}
	for i := range cookies {
		writer.WriteString(cookies[i].Name)
		writer.WriteString(" ")
		writer.WriteString(cookies[i].Value)
		writer.WriteString("\n")
	}
}
