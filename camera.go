package main

import (
	"github.com/antonholmquist/jason"
	"strings"
)

func findCamera(metadata []*jason.Object) (string, string) {
	make := ""
	model := ""
	for i := 0; i < len(metadata); i++ {
		name, err := metadata[i].GetString("name")
		if err != nil {
			panic(err)
		}
		if name == "Make" {
			value, err := metadata[i].GetString("value")
			if err == nil {
				make = strings.Trim(value, " \n")
			}
		} else if name == "Model" {
			value, err := metadata[i].GetString("value")
			if err == nil {
				model = strings.Trim(value, " \n")
			}
		} else if name == "metadata" {
			// MediaWiki can return strange embedded metadata
			// arrays for PNG files.
			obj, err := metadata[i].GetObjectArray("value")
			// Ignore if not an object array.
			if err == nil {
				tmake, tmodel := findCamera(obj)
				if tmake != "" {
					make = tmake
				}
				if tmodel != "" {
					model = tmodel
				}
			}
		}
	}
	return make, model
}

// Return device make/model from json imageinfo object.
func extractCamera(imageinfo *jason.Object) (string, string) {
	metadata, err := imageinfo.GetObjectArray("commonmetadata")
	if err != nil {
		// metadata is null in some cases
		return "", ""
	} else {
		return findCamera(metadata)
	}
}
