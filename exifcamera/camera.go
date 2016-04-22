package exifcamera

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"fmt"
	"github.com/antonholmquist/jason"
	"github.com/garyhouston/takenwith/mwlib"
	"strings"
)

func requestExif(pages []string, client *mwclient.Client) *jason.Object {
	params := params.Values{
		"action":    "query",
		"titles":    mwlib.MakeTitleString(pages),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
		"redirects": "", // follow redirects
		"continue":  "",
	}
	json, err := client.Get(params)
	if err != nil {
		panic(err)
	}
	return json
}

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
func ExtractCamera(imageinfo *jason.Object) (string, string) {
	metadata, err := imageinfo.GetObjectArray("commonmetadata")
	if err != nil {
		// metadata is null in some cases
		return "", ""
	} else {
		return findCamera(metadata)
	}
}

type FileCamera struct {
	Title string
	Make  string
	Model string
}

// Return the camera manufacturer and model from Exif. Also follow any
// redirects and return the final page name.
func GetCameraInfo(pageReq []string, client *mwclient.Client) []FileCamera {
	exif := requestExif(pageReq, client)
	pages, err := exif.GetObject("query", "pages")
	if err != nil {
		panic(err)
	}
	pageMap := pages.Map()
	pageArray := make([]FileCamera, 0, len(pageMap))
	idx := 0
	for pageId, page := range pageMap {
		pageObj, err := page.Object()
		if err != nil {
			panic(err)
		}
		title, err := pageObj.GetString("title")
		if err != nil {
			panic(err)
		}
		if pageId == "-1" {
			fmt.Println(title)
			fmt.Println("File does not exist, possibly deleted.")
			continue
		}
		make := ""
		model := ""
		imageinfo, err := pageObj.GetObjectArray("imageinfo")
		if err == nil {
			make, model = ExtractCamera(imageinfo[0])
		}
		pageArray = pageArray[0 : idx+1]
		pageArray[idx] = FileCamera{title, make, model}
		idx++
	}
	return pageArray
}
