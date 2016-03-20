package canons100

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"fmt"
	"github.com/antonholmquist/jason"
	"github.com/garyhouston/takenwith/exifcamera"
	"strings"
)

// This module recategorises files that the main bot has dumped into
// "Category:Taken with unidentified Canon PowerShot S100" and
// "Category:Taken with unidentified Canon PowerShot S110".

type CatInfo struct {
	ExifModel         string
	UnidCategory      string
	PowershotCategory string
	IxusCategory      string
}

func moveFile(file string, powershot bool, cat CatInfo, client *mwclient.Client, verbose bool) {
	var target string
	var reason string
	if powershot {
		target = cat.PowershotCategory
		reason = "since Exif contains ISO speed rating"
	} else {
		target = cat.IxusCategory
		reason = "since Exif lacks ISO speed rating"
	}
	if verbose {
		fmt.Println("moving", file, "from", cat.UnidCategory, "to", target)
	}

	// There's a small chance that saving a page may fail due to an
	// edit conflict. It also occasionally fails with
	// "badtoken: Invalid token" for unknown reason. Try up
	// to 3 times before giving up.
	var saveError error
	for i := 0; i < 3; i++ {
		text, timestamp, err := client.GetPageByName(file)
		if err != nil {
			panic(fmt.Sprintf("%v %v", file, err))
		}
		newText := strings.Replace(text, cat.UnidCategory, target, -1)
		editcfg := map[string]string{
			"action":        "edit",
			"title":         file,
			"text":          newText,
			"summary":       "moved from [[" + cat.UnidCategory + "]] to [[" + target + "]] " + reason,
			"minor":         "",
			"bot":           "",
			"basetimestamp": timestamp,
		}
		saveError = client.Edit(editcfg)
		if saveError == nil {
			break
		}
	}
}

func checkSpeedRatings(metadata []*jason.Object) bool {
	for i := 0; i < len(metadata); i++ {
		name, err := metadata[i].GetString("name")
		if err != nil {
			panic(err)
		}
		if name == "ISOSpeedRatings" {
			return true
		}
		if name == "metadata" {
			// MediaWiki can return strange embedded metadata
			// arrays for PNG files.
			// E.g., File:Plaza_in_Front_of_BEXCO.png
			obj, err := metadata[i].GetObjectArray("value")
			// Ignore if not an object array.
			if err == nil && checkSpeedRatings(obj) {
				return true
			}
		}
	}
	return false
}

func processFile(pageObj *jason.Object, cat CatInfo, client *mwclient.Client, verbose bool) {
	title, err := pageObj.GetString("title")
	if err != nil {
		panic(err)
	}
	imageinfo, err := pageObj.GetObjectArray("imageinfo")
	if err == nil {
		_, model := exifcamera.ExtractCamera(imageinfo[0])
		if model != cat.ExifModel {
			fmt.Println(title)
			fmt.Println("Skipping due to wrong model in Exif")
			return
		}
		metadata, err := imageinfo[0].GetObjectArray("metadata")
		if err != nil {
			panic(err)
		}
		if checkSpeedRatings(metadata) {
			moveFile(title, true, cat, client, verbose)
		} else {
			moveFile(title, false, cat, client, verbose)
		}
	}
}

func ProcessCategory(cat CatInfo, client *mwclient.Client, verbose bool) {
	params := params.Values{
		"generator": "categorymembers",
		"gcmtitle":  cat.UnidCategory,
		"gcmtype":   "file",
		"gcmsort":   "sortkey",
		"gcmlimit":  "100", // Maximum files per batch, API allows 5k with bot flag.
		"prop":      "imageinfo",
		"iiprop":    "metadata",
	}
	query := client.NewQuery(params)
	for query.Next() {
		json := query.Resp()
		pages, err := json.GetObject("query", "pages")
		if err != nil {
			// empty category
			return
		}
		pagesMap := pages.Map()
		if len(pagesMap) > 0 {
			for _, page := range pagesMap {
				pageObj, err := page.Object()
				if err != nil {
					panic(err)
				}
				processFile(pageObj, cat, client, verbose)
			}
		}
	}
}
