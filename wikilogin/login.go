package main

import (
	"bufio"
	mwclient "cgt.name/pkg/go-mwclient"
	"flag"
	"fmt"
	"github.com/garyhouston/takenwith/mwlib"
	"github.com/vharitonsky/iniflags"
	"log"
	"os"
)

// Flags are just to get the user details, usually from the configuration file.
func getUser() string {
	iniflags.SetConfigFile(mwlib.GetWorkingDir() + "/wikilogin.conf")
	var user string
	flag.StringVar(&user, "user", "nobody@example.com", "Operator's email address or Wiki user name.")
	iniflags.Parse()
	return user
}

// This login program must be run before using the main bot. It saves
// cookies into a file in the bot's directory.
func main() {
	userinfo := getUser()
	if len(os.Args) != 1 {
		log.Fatal("Usage: ", os.Args[0], " -help: display options")
	}
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
