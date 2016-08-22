package main

import (
	"github.com/antonholmquist/jason"
	"strconv"
)

// bool: whether metadata (Exif) contains an ISOSpeedRatings field
// int: year from photo date in metadata, or 0 if not found.
func checkExifFields(metadata []*jason.Object) (bool, int) {
	hasRatings := false
	year := 0
	for i := 0; i < len(metadata); i++ {
		name, err := metadata[i].GetString("name")
		if err != nil {
			panic(err)
		}
		if name == "DateTimeOriginal" {
			value, err := metadata[i].GetString("value")
			if err != nil {
				panic(err)
			}
			// on error, year will remain 0.
			if len(value) > 3 {
				year, _ = strconv.Atoi(value[0:4])
			}
		}
		if name == "ISOSpeedRatings" {
			hasRatings = true
		}
		if name == "metadata" {
			// MediaWiki can return strange embedded metadata
			// arrays for PNG files.
			// E.g., File:Plaza_in_Front_of_BEXCO.png
			obj, err := metadata[i].GetObjectArray("value")
			// Ignore if not an object array.
			if err == nil {
				return checkExifFields(obj)
			}
		}
	}
	return hasRatings, year
}

func mapCanonS100(imageinfo []*jason.Object) string {
	metadata, err := imageinfo[0].GetObjectArray("commonmetadata")
	if err != nil {
		panic(err)
	}
	hasRatings, year := checkExifFields(metadata)
	if hasRatings {
		return "Category:Taken with Canon PowerShot S100"
	} else {
		if year == 0 || year > 2010 {
			// May be PowerShot S100 (released in 2011) with missing Exif fields.
			return "Category:Taken with unidentified Canon PowerShot S100"
		} else {
			return "Category:Taken with Canon Digital IXUS"
		}
	}
}

func mapCanonS110(imageinfo []*jason.Object) string {
	metadata, err := imageinfo[0].GetObjectArray("commonmetadata")
	if err != nil {
		panic(err)
	}
	hasRatings, year := checkExifFields(metadata)
	if hasRatings {
		return "Category:Taken with Canon PowerShot S110"
	} else {
		if year == 0 || year > 2011 {
			// May be PowerShot S110 (released in 2012) with missing Exif fields.
			return "Category:Taken with unidentified Canon PowerShot S110"
		} else {
			return "Category:Taken with Canon Digital IXUS v"
		}
	}
}
