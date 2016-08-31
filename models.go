package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

func convertCategory(field string) string {
	if strings.HasPrefix(field, "Category:") {
		// use name as-is.
		return field
	} else if strings.HasPrefix(field, "skip ") {
		// keyword for entry to be ignored.
		return field
	} else if strings.HasPrefix(field, "Taken with") {
		// avoid accidental "Taken with Taken with".
		panic(fmt.Sprintf("Bad record in category mapping file: %v", field))
	} else {
		return "Category:Taken with " + field
	}
}

// fill map with relations of makemodel -> Commons category
func fillCategoryMap(mappingFile string) map[string]string {
	file, err := os.Open(mappingFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReader(file))
	var categories = make(map[string]string)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		categories[record[0]+record[1]] = convertCategory(record[2])
	}
	return categories
}

// Fill the complete set of relevant Commons Categories.
func fillCategories(categoryMap map[string]string, exceptionFile string) map[string]bool {
	var categories = make(map[string]bool)
	for _, v := range categoryMap {
		categories[v] = true
	}
	// Read the categories that don't start with "Taken ".
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
