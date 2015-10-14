package exifcamera

import(
	"fmt"
	"github.com/antonholmquist/jason"
	"cgt.name/pkg/go-mwclient/params"
	mwclient "cgt.name/pkg/go-mwclient"
	"github.com/garyhouston/takenwith/mwlib"
	"strings"
)

func requestExif(pages []string, client *mwclient.Client) (*jason.Object) {
	params := params.Values {
		"action": "query",
		"titles": mwlib.MakeTitleString(pages),
		"prop": "imageinfo",
		"iiprop": "metadata",
		"redirects" : "",  // follow redirects
		"continue": "",
	}
	json, err := client.Get(params)
	if err != nil {
		panic(err)
	}
	return json
}

// Return device make/model from json imageinfo object.
func ExtractCamera(imageinfo *jason.Object) (string, string) {
	metadata, err := imageinfo.GetObjectArray("metadata")
	if err != nil {
		// metadata is null in some cases
		return "", ""
	}
	make := ""
	model := ""
	for i := 0; i < len(metadata); i++ {
		name, err := metadata[i].GetString("name")
		if err != nil {
			panic(err)
		}
		if name == "Make" {
			value, err := metadata[i].GetString("value")
			if err != nil {
				panic(err)
			}
			make = strings.Trim(value, " ")
		} else if name == "Model" {
			value, err := metadata[i].GetString("value")
			if err != nil {
				panic(err)
			}
			model = strings.Trim(value, " ")
		}
	}
	return make, model
}

type FileCamera struct {
	Title string
	Make string
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
		pageArray = pageArray[0:idx + 1]
		pageArray[idx] = FileCamera{title, make, model}
		idx++
	}
	return pageArray
}
