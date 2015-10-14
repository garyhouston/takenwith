package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"encoding/gob"
	"github.com/garyhouston/takenwith/mwlib"
	"os"
)

func getCountsFile() string {
	return mwlib.GetWorkingDir() + "/catcounts"
}

// Remove counts smaller than the given value
func removeSmallCounts(catCounts map[string]int32, limit int32) {
	for idx := range catCounts {
		if catCounts[idx] < limit {
			delete(catCounts, idx)
		}
	}
}

func loadCatCounts() map[string]int32 {
	var catCounts map[string]int32
	file, err := os.Open(getCountsFile())
	if err == nil {
		decoder := gob.NewDecoder(file)
		err := decoder.Decode(&catCounts)
		if err != nil {
			panic(err)
		}
		err = file.Close()
		if err != nil {
			panic(err)
		}
	} else if os.IsNotExist(err) {
		catCounts = make(map[string]int32)
	} else {
		panic(err)
	}
	return catCounts
}

func writeCatCounts(catCounts map[string]int32) {
	countsFile := getCountsFile()
	tmpCountsFile := countsFile + ".tmp"
	file, err := os.Create(tmpCountsFile)
	if err != nil {
		panic(err)
	}
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(catCounts)
	if err != nil {
		panic(err)
	}
	err = file.Close()
	if err != nil {
		panic(err)
	}
	err = os.Rename(tmpCountsFile, countsFile)
	if err != nil {
		panic(err)
	}
}

// Add entries to catCounts for the specified categories.
func setCatCounts(categories []string, client *mwclient.Client, catCounts map[string]int32) {
	cats, counts := catNumFiles(categories, client)
	for i := range cats {
		catCounts[cats[i]] = counts[i]
	}
	writeCatCounts(catCounts)
}

func incCatCount(category string, catCounts map[string]int32) {
	catCounts[category] = catCounts[category] + 1
        writeCatCounts(catCounts)
}
