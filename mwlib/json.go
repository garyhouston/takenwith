package mwlib

import (
	"github.com/antonholmquist/jason"
)

// Return page data from JSON, or nil if page is "-1", i.e., not found.
// Assumes only a single page was requested.
func GetJsonPage(json *jason.Object) *jason.Object {
	pages, err := json.GetObject("query", "pages")
	if err != nil {
		panic(err)
	}
	for key, value := range pages.Map() {
		if key == "-1" {
			return nil
		} else {
			valueObj, err := value.Object()
			if err != nil {
				panic(err)
			}
			return valueObj
		}
	}
	panic("getJsonPage fallthrough")
}

// Write an array of titles into a piped request string.
func MakeTitleString(titles []string) string {
	var result string = ""
	for i := range titles {
		if i > 0 {
			result += "|"
		}
		result += titles[i]
	}
	return result
}
