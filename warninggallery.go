package main

import (
	"bytes"
	mwclient "cgt.name/pkg/go-mwclient"
	"fmt"
	"sort"
	"strings"
)

type warning struct {
	title        string
	warning      string
	warningLower string // Lower case version for string-insensitive sort.
}

type warnings []warning

// Sort interface functions.
func (warnings warnings) Len() int {
	return len(warnings)
}

func (warnings warnings) Less(i, j int) bool {
	return warnings[i].warningLower < warnings[j].warningLower
}

func (warnings warnings) Swap(i, j int) {
	warnings[i], warnings[j] = warnings[j], warnings[i]
}

func (warnings *warnings) Append(files []fileData) {
	for i := range files {
		if files[i].warning != "" {
			trimmed := strings.TrimSpace(files[i].warning)
			*warnings = append(*warnings, warning{files[i].title, trimmed, strings.ToLower(trimmed)})
		}
	}
}

// Create a gallery showing all the files with warnings. Page must already
// exist and will be replaced.
func (warnings warnings) createGallery(gallery string, client *mwclient.Client) {
	var saveError error
	sort.Sort(warnings)
	for i := 0; i < 3; i++ {
		_, timestamp, err := client.GetPageByName(gallery)
		if err != nil {
			panic(fmt.Sprintf("%v %v", gallery, err))
		}
		// Blank the page and create a fresh gallery
		var buffer bytes.Buffer
		buffer.WriteString("<gallery>\n")
		for w := range warnings {
			buffer.WriteString(warnings[w].title)
			buffer.WriteByte('|')
			// Replace problematic text
			desc := warnings[w].warning
			if strings.Contains(desc, "http:") {
				desc = "URL omitted" // Some URLs are blacklisted and gallery won't save
			}
			desc = strings.Replace(desc, "|", "<nowiki>|</nowiki>", -1)
			desc = strings.Replace(desc, "\n", "<br>", -1)
			if len(desc) > 200 {
				buffer.WriteString(desc[0:200])
				buffer.WriteString("...")
			} else {
				buffer.WriteString(desc)
			}
			buffer.WriteByte('\n')
		}
		buffer.WriteString("</gallery>")
		editcfg := map[string]string{
			"action":        "edit",
			"title":         gallery,
			"text":          buffer.String(),
			"bot":           "",
			"basetimestamp": timestamp,
		}
		saveError = client.Edit(editcfg)
		if saveError != nil && strings.Contains(saveError.Error(), "edit successful, but did not change page") {
			saveError = nil
		}
		if saveError == nil {
			break
		}
	}
	if saveError != nil {
		panic(fmt.Sprintf("Failed to save %v %v", gallery, saveError))
	}
}
