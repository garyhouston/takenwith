package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"github.com/garyhouston/takenwith/mwlib"
)

// Examine the specified categories in the Wiki. For each one that actually
// exists, return the number of files that it contains. The result arrays
// give category names (in arbitrary order) and the corresponding count,
// and will have fewer entries than the input array if some categories
// were duplicated or didn't exist.
func catNumFiles(categories []string, client *mwclient.Client) ([]string, []int32) {
	params := params.Values{
		"action": "query",
		"titles": mwlib.MakeTitleString(categories),
		"prop":   "categoryinfo",
	}
	json, err := client.Post(params) // Get may fail on long queries.
	if err != nil {
		panic(err)
	}
	pages, err := json.GetObject("query", "pages")
	if err != nil {
		panic(err)
	}
	pageMap := pages.Map()
	var resultCats = make([]string, len(pageMap))
	var resultCounts = make([]int32, len(pageMap))
	var idx int32 = 0
	for pageId, page := range pageMap {
		pageObj, err := page.Object()
		if err != nil {
			panic(err)
		}
		if pageId[0] != '-' {
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
	resultCats = resultCats[0:idx]
	resultCounts = resultCounts[0:idx]
	return resultCats, resultCounts
}
