package main

import (
	"bufio"
	mwclient "cgt.name/pkg/go-mwclient"
	"fmt"
	"github.com/garyhouston/takenwith/mwlib"
	goflags "github.com/jessevdk/go-flags"
	"log"
	"os"
)

// Get the user email address / Wiki name, from command line or environment variable.
func getOperator() string {
	var flags struct {
		Operator string `long:"user" env:"takenwith_user" description:"Operator's email address or Wiki user name"`
	}
	parser := goflags.NewParser(&flags, goflags.HelpFlag)
	args, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
	if len(args) != 0 {
		log.Fatal("Unexpected argument.")
	}
	return flags.Operator
}

// This login program must be run before using the main bot. It saves
// cookies into a file in the bot's directory.
func main() {
	operator := getOperator()
	if operator == "" {
		log.Fatal("Operator email / username not set.")
	}
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "wikilogin "+operator)
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
