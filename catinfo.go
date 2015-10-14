package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"github.com/garyhouston/takenwith/mwlib"
	"cgt.name/pkg/go-mwclient/params"
)

// Examine the specified categories in the Wiki. For each one that actually
// exists, return the number of files that it contains. The result arrays
// give category names (in arbitrary order) and the corresponding count,
// and will have fewer entries than the input array if some categories
// were duplicated or didn't exist.
func catNumFiles(categories []string, client *mwclient.Client) ([]string, []int32) {
	params := params.Values {
		"action": "query",
		"titles": mwlib.MakeTitleString(categories),
		"prop" : "categoryinfo",
	}
	json, err := client.Get(params)
	if err != nil {
		panic(err)
	}
	pages, err := json.GetObject("query", "pages")
	if err != nil {
		panic(err)
	}
	pageMap := pages.Map()
	var resultCats = make([]string, 0, len(pageMap))
	var resultCounts = make([]int32, 0, len(pageMap))
	var idx int32 = 0
	for pageId, page := range pageMap {
		pageObj, err := page.Object()
		if err != nil {
			panic(err)
		}
		if pageId[0] != '-' {
			resultCats = resultCats[0 : idx + 1]
			resultCounts = resultCounts[0 : idx + 1]
			resultCats[idx], err = pageObj.GetString("title")
			if err != nil {
				panic(err)
			}
			info, err := pageObj.GetObject("categoryinfo")
			// An error here means that the category is probably
			// empty, so just leave count at 0.
			if err == nil {
				files, err := info.GetInt64("files")
				if err != nil {
					panic(err)
				}
				resultCounts[idx] = int32(files)
			}
			idx++
		}
	}
	return resultCats, resultCounts
}
