package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"fmt"
	"github.com/antonholmquist/jason"
	"github.com/garyhouston/takenwith/mwlib"
)

func requestCategories(page string, client *mwclient.Client) *jason.Object {
	params := params.Values{
		"action":  "query",
		"titles":  page,
		"prop":    "categories",
		"cllimit": "max",
	}
	json, err := client.Get(params)
	if err != nil {
		panic(err)
	}
	return json
}

// Given an array of page titles, return a mapping from page title to the array
// of categories which the page is a member of.
// If the page doesn't exist, no entry is added to the map.
// If the page has no categories, it will map to nil.
func getPageCategories(pages []string, client *mwclient.Client) map[string][]string {
	params := params.Values{
		"action":   "query",
		"titles":   mwlib.MakeTitleString(pages),
		"prop":     "categories",
		"cllimit":  "max",
		"continue": "",
	}
	json, err := client.Post(params) // Get may fail on long queries.
	if err != nil {
		fmt.Println(params)
		panic(err)
	}
	pageData, err := json.GetObject("query", "pages")
	if err != nil {
		panic(err)
	}
	pageMap := pageData.Map()
	result := make(map[string][]string)
	for pageId, page := range pageMap {
		pageObj, err := page.Object()
		if err != nil {
			panic(err)
		}
		title, err := pageObj.GetString("title")
		if err != nil {
			panic(err)
		}
		if pageId[0] == '-' {
			fmt.Println(title)
			fmt.Println("File does not exist, possibly deleted.")
			continue
		}
		categories, err := pageObj.GetObjectArray("categories")
		if err != nil {
			// Presumably the page has no categories.
			result[title] = nil
			continue
		}
		catArray := make([]string, len(categories))
		for i := range categories {
			catArray[i], err = categories[i].GetString("title")
			if err != nil {
				panic(err)
			}
		}
		result[title] = catArray
	}
	return result
}
