package main

import (
	"bufio"
	goflags "github.com/jessevdk/go-flags"
	mwclient "cgt.name/pkg/go-mwclient"
	"fmt"
	"github.com/garyhouston/takenwith/mwlib"
	"log"
	"os"
)

// Get the user email address / Wiki name, from command line or environment variable.
func getUser() string {
	var flags struct {
		User  string `long:"user" env:"takenwith_user" description:"Operator's email address or Wiki user name" default:"nobody@example.com"`
	}
	parser := goflags.NewParser(&flags, goflags.HelpFlag)
	args, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
	if len(args) != 0 {
		log.Fatal("Unexpected argument.")
	}
	return flags.User
}

// This login program must be run before using the main bot. It saves
// cookies into a file in the bot's directory.
func main() {
	userinfo := getUser()
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "wikilogin "+userinfo)
	if err != nil {
		panic(err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Username: ")
	scanner.Scan()
	user := scanner.Text()
	fmt.Print("Password: ")
	scanner.Scan()
	password := scanner.Text()

	client.Maxlag.On = true

	err = client.Login(user, password)
	if err != nil {
		log.Fatal(err)
	}
	mwlib.WriteCookies(client.DumpCookies())
}
