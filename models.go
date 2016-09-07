package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"regexp"
	"strings"
)

func readCSV(mappingFile string, convert func(record []string)) {
	file, err := os.Open(mappingFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReader(file))
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		convert(record)
	}
}

func convertTarget(in string) string {
	var out string
	if strings.HasPrefix(in, "Category:") {
		// use name as-is.
		out = in
	} else if strings.HasPrefix(in, "Taken with") {
		// avoid accidental "Taken with Taken with".
		panic("Bad record in mapping file: " + in)
	} else {
		out = "Category:Taken with " + in
	}
	return out
}

// Fill map with relations of makemodel -> Commons category
func fillCategoryMap(mappingFile string) map[string]string {
	categories := make(map[string]string)
	convert := func(record []string) {
		categories[record[0]+record[1]] = convertTarget(record[2])
	}
	readCSV(mappingFile, convert)
	return categories
}

type catRegex struct {
	regex  *regexp.Regexp
	target string
}

// Read regular expressions for category matches.
func fillRegex(regexFile string) []catRegex {
	regexes := make([]catRegex, 0, 200)
	convert := func(record []string) {
		regex, err := regexp.Compile(record[0])
		if err != nil {
			panic(err)
		}
		regexes = append(regexes, catRegex{regex, convertTarget(record[1])})
	}
	readCSV(regexFile, convert)
	return regexes
}

// Fill the complete set of relevant Commons Categories.
func fillCategories(categoryMap map[string]string, exceptionFile string) map[string]bool {
	categories := make(map[string]bool)
	for _, v := range categoryMap {
		categories[v] = true
	}
	// Add the categories that aren't catmapping targets.
	file, err := os.Open(exceptionFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			panic(err)
		}
		categories["Category:"+scanner.Text()] = true
	}
	return categories
}
