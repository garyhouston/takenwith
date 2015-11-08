package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"flag"
	"fmt"
	"github.com/garyhouston/takenwith/canons100"
	"github.com/garyhouston/takenwith/exifcamera"
	mwlib "github.com/garyhouston/takenwith/mwlib"
	"github.com/vharitonsky/iniflags"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Find the position in the text following the last category.
func afterCategories(page string, text *string) int {
	regex, error := regexp.Compile("\\[\\[[Cc]ategory:[^\\]]*\\]\\]")
	if error != nil {
		panic(error)
	}
	matches := regex.FindAllIndex([]byte(*text), -1)
	noMatches := len(matches)
	if noMatches == 0 {
		// No existing categories, use the last byte of the page.
		return len(*text)
	}
	lastMatch := matches[len(matches)-1]
	return lastMatch[1]
}

func addCategory(page string, category string, client *mwclient.Client) {
	// There's a small change that saving a page may fail due to an
	// edit conflict. It also occasionally fails with
	// "badtoken: Invalid token" for unknown reason. Try up
	// to 3 times before giving up.
	var saveError error
	for i := 0; i < 3; i++ {
		text, timestamp, err := client.GetPageByName(page)
		if err != nil {
			panic(fmt.Sprintf("%v %v", page, err))
		}
		last := afterCategories(page, &text)
		text = text[0:last] + "\n[[" + category + "]]" + text[last:]
		editcfg := map[string]string{
			"action":        "edit",
			"title":         page,
			"text":          text,
			"summary":       "added [[" + category + "]]",
			"minor":         "",
			"bot":           "",
			"basetimestamp": timestamp,
		}
		saveError = client.Edit(editcfg)
		if saveError == nil {
			break
		}

	}
	if saveError != nil {
		fmt.Sprintf("Failed to save %v %v", page, saveError)
	}
}

type fileTarget struct {
	title    string
	category string
}

func addCategories(pages []fileTarget, client *mwclient.Client, verbose bool, catFileLimit int32, allCategories map[string]bool, catCounts map[string]int32) {
	for i := range pages {
		// The cat size limit needs to be checked again, since adding
		// previous files in the batch may have pushed it over the
		// limit.
		if catFileLimit > 0 && catCounts[pages[i].category] >= catFileLimit {
			if verbose {
				fmt.Println(pages[i].title)
				fmt.Println("Already populated:", pages[i].category)
			}
		} else {
			// Identifying emtpy categories may help identify
			// when we are adding a file to a redirect page
			// for a renamed category. Ideally this would be done
			// regardless of catFileLimit, but counts aren't
			// maintained if it's zero.
			if catFileLimit > 0 && catCounts[pages[i].category] == 0 {
				fmt.Println(pages[i].title)
				fmt.Println("Adding to empty", pages[i].category)
			} else if verbose {
				fmt.Println(pages[i].title)
				fmt.Println("Adding to", pages[i].category, " (", catCounts[pages[i].category], " files)")
			}
			addCategory(pages[i].title, pages[i].category, client)
			if catFileLimit > 0 {
				incCatCount(pages[i].category, catCounts)
			}
		}
	}
}

// Remove files where the category already has more than catFileLimt members.
func filterCatLimit(cats []fileTarget, client *mwclient.Client, verbose bool, catFileLimit int32, catCounts map[string]int32) []fileTarget {
	// Identify categories where the size isn't already cached.
	lookup := make([]string, 0, len(cats))
	lookupIdx := 0
	for i := range cats {
		_, found := catCounts[cats[i].category]
		if !found {
			lookup = lookup[0 : lookupIdx+1]
			lookup[lookupIdx] = cats[i].category
			lookupIdx++
		}
	}
	// Try to cache the uncached
	if lookupIdx > 0 {
		setCatCounts(lookup, client, catCounts)
	}
	// Filter the category list to remove those where the category is
	// missing or already populated.
	result := make([]fileTarget, 0, len(cats))
	resultIdx := 0
	for i := range cats {
		count, found := catCounts[cats[i].category]
		if !found {
			fmt.Println(cats[i].title)
			fmt.Println("Mapped category doesn't exist:", cats[i].category)
			continue
		}
		if count >= catFileLimit {
			if verbose {
				fmt.Println(cats[i].title)
				fmt.Println("Already populated:", cats[i].category)
			}
			continue
		}
		result = result[0 : resultIdx+1]
		result[resultIdx] = cats[i]
		resultIdx++
	}
	return result
}

func filterFiles(pages []exifcamera.FileCamera, client *mwclient.Client, verbose bool, categoryMap map[string]string) []fileTarget {
	count := 0
	result := make([]fileTarget, 0, len(pages))
	for i := range pages {
		if pages[i].Make == "" && pages[i].Model == "" {
			if verbose {
				fmt.Println(pages[i].Title)
				fmt.Println("No camera details in Exif")
			}
			continue
		}
		var catMapped string
		catMapped, ok := categoryMap[pages[i].Make+pages[i].Model]
		if !ok {
			fmt.Println(pages[i].Title)
			fmt.Printf("No category for %v,%v\n", pages[i].Make, pages[i].Model)
			continue
		}
		if strings.HasPrefix(catMapped, "skip ") {
			if verbose {
				fmt.Println(pages[i].Title)
				fmt.Println("Skipping", pages[i].Make, pages[i].Model)
			}
			continue
		}
		result = result[0 : count+1]
		result[count] = fileTarget{pages[i].Title, catMapped}
		count++
	}
	return result
}

// Remove files which are already in a relevant category.
func filterCategories(files []fileTarget, client *mwclient.Client, verbose bool, allCategories map[string]bool) []fileTarget {
	fileArray := make([]string, len(files))
	for i := range files {
		fileArray[i] = files[i].title
	}
	fileCats := getPageCategories(fileArray, client)
	result := make([]fileTarget, 0, len(files))
	resultIdx := 0
	for i := range files {
		found := false
		cats := fileCats[files[i].title]
		for j := range cats {
			if files[i].category == cats[j] {
				if verbose {
					fmt.Println(files[i].title)
					fmt.Println("Already in mapped:", files[i].category)
				}
				found = true
				break
			}
			_, found = allCategories[cats[j]]
			if found {
				if verbose {
					fmt.Println(files[i].title)
					fmt.Println("Already in known:", cats[j])
				}
				break
			}
			if strings.HasPrefix(cats[j], "Category:Taken ") {
				fmt.Println(files[i].title)
				fmt.Println("Already in an unknown 'Taken ':", cats[j])
				found = true
				break
			}
		}
		if !found {
			result = result[0 : resultIdx+1]
			result[resultIdx] = files[i]
			resultIdx++
		}
	}
	return result
}

func processFiles(fileArray []exifcamera.FileCamera, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	selected := filterFiles(fileArray, client, flags.verbose, categoryMap)
	if len(selected) == 0 {
		return
	}
	if flags.catFileLimit > 0 {
		selected = filterCatLimit(selected, client, flags.verbose, flags.catFileLimit, catCounts)
		if len(selected) == 0 {
			return
		}
	}
	selected = filterCategories(selected, client, flags.verbose, allCategories)
	if len(selected) == 0 {
		return
	}
	addCategories(selected, client, flags.verbose, flags.catFileLimit, allCategories, catCounts)
}

func processGenerator(params params.Values, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	query := client.NewQuery(params)
	for query.Next() {
		json := query.Resp()
		pages, err := json.GetObject("query", "pages")
		if err != nil {
			panic(err)
		}
		pagesMap := pages.Map()
		if len(pagesMap) > 0 {
			pageArray := make([]exifcamera.FileCamera, len(pagesMap))
			idx := 0
			for _, page := range pagesMap {
				pageObj, err := page.Object()
				if err != nil {
					panic(err)
				}
				title, err := pageObj.GetString("title")
				if err != nil {
					panic(err)
				}
				imageinfo, err := pageObj.GetObjectArray("imageinfo")
				if err != nil {
					pageArray[idx] = exifcamera.FileCamera{title, "", ""}
				} else {
					make, model := exifcamera.ExtractCamera(imageinfo[0])
					pageArray[idx] = exifcamera.FileCamera{title, make, model}
				}
				idx++
			}
			processFiles(pageArray, client, flags, categoryMap, allCategories, catCounts)
		}
	}
	if query.Err() != nil {
		panic(query.Err())
	}
}

func processUser(user string, timestamp string, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	params := params.Values{
		"generator": "allimages",
		"gaiuser":   strings.TrimPrefix(user, "User:"),
		"gaisort":   "timestamp",
		"gaidir":    "descending",
		"gaistart":  timestamp,
		"gailimit":  strconv.Itoa(flags.batchSize),
		"prop":      "imageinfo",
		"iiprop":    "metadata",
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts)
}

func processCategory(category string, startKey string, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	params := params.Values{
		"generator":             "categorymembers",
		"gcmtitle":              category,
		"gcmtype":               "file",
		"gcmsort":               "sortkey",
		"gcmstartsortkeyprefix": startKey,
		"gcmlimit":              strconv.Itoa(flags.batchSize),
		"prop":                  "imageinfo",
		"iiprop":                "metadata",
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts)
}

func processRandom(client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	batchSize := 20 // max accepted by random API.
	if flags.batchSize < 20 {
		batchSize = flags.batchSize
	}
	for {
		params := params.Values{
			"generator":    "random",
			"grnnamespace": "6", // namespace 6 for files on Commons.
			"grnlimit":     strconv.Itoa(batchSize),
			"prop":         "imageinfo",
			"iiprop":       "metadata",
		}
		processGenerator(params, client, flags, categoryMap, allCategories, catCounts)
	}
}

func processSequence(forward bool, timestamp string, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	var direction string
	if forward {
		direction = "ascending"
	} else {
		direction = "descending"
	}
	params := params.Values{
		"generator": "allimages",
		"gaisort":   "timestamp",
		"gaidir":    direction,
		"gaistart":  timestamp,
		"gailimit":  strconv.Itoa(flags.batchSize),
		"prop":      "imageinfo",
		"iiprop":    "metadata",
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts)
}

func processOneFile(page string, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32) {
	pageArray := make([]string, 1)
	pageArray[0] = page
	camArray := exifcamera.GetCameraInfo(pageArray, client)
	processFiles(camArray, client, flags, categoryMap, allCategories, catCounts)
}

type flags struct {
	verbose      bool
	catFileLimit int32
	user         string
	batchSize    int
}

func parseFlags() flags {
	iniflags.SetConfigFile(mwlib.GetWorkingDir() + "/takenwith.conf")
	var flags flags
	flag.BoolVar(&flags.verbose, "verbose", false, "Print action for every file")
	var catFileLimit int
	flag.IntVar(&catFileLimit, "catFileLimit", 100, "Don't add to categories with at least this many files. No limit if zero.")
	flag.StringVar(&flags.user, "user", "nobody@example.com", "Operator's email address or Wiki user name.")
	flag.IntVar(&flags.batchSize, "batchSize", 100, "Number of files to process per server request.")
	iniflags.Parse()
	flags.catFileLimit = int32(catFileLimit)
	return flags
}

func usage(progName string) {
	log.Fatal("Usage: \n", progName, " File:f\n", progName, " User:u [timestamp]\n", progName, " Category:c [sort key prefix]\n", progName, " Random\n", progName, " Forward timestamp\n", progName, " Back timestamp\n", progName, " CanonS100\n", "-help: display options.")
}

func main() {
	flags := parseFlags()
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "takenwith "+flags.user)
	if err != nil {
		panic(err)
	}
	client.Maxlag.On = true

	cookies := mwlib.ReadCookies()
	client.LoadCookies(cookies)

	categoryMap := fillCategoryMap() // makemodel -> category

	// All known categories, including subcategories that don't start
	// with "Taken with".
	allCategories := fillCategories(categoryMap)

	// Counts of files in each category.
	catCounts := loadCatCounts()

	if flags.catFileLimit > 0 {
		// Remove category counts for categories that we may need to update,
		// since the count may have changed since the bot last ran.
		removeSmallCounts(catCounts, flags.catFileLimit)
	}

	args := flag.Args()
	numArgs := len(args)
	if numArgs == 0 || numArgs > 2 {
		usage(os.Args[0])
	}
	if strings.HasPrefix(args[0], "File:") {
		if numArgs > 1 {
			usage(os.Args[0])
		}
		processOneFile(args[0], client, flags, categoryMap, allCategories, catCounts)
	} else if strings.HasPrefix(args[0], "User:") {
		var timestamp string
		if numArgs == 2 {
			timestamp = args[1]
		} else {
			timestamp = "2099-01-01T00:00:00Z"
		}
		processUser(args[0], timestamp, client, flags, categoryMap, allCategories, catCounts)
	} else if strings.HasPrefix(args[0], "Category:") {
		var startKey string
		if numArgs == 2 {
			startKey = args[1]
		} else {
			startKey = ""
		}
		processCategory(args[0], startKey, client, flags, categoryMap, allCategories, catCounts)
	} else if args[0] == "Random" {
		if numArgs > 1 {
			usage(os.Args[0])
		}
		processRandom(client, flags, categoryMap, allCategories, catCounts)
	} else if args[0] == "Forward" {
		if numArgs != 2 {
			usage(os.Args[0])
		}
		processSequence(true, args[1], client, flags, categoryMap, allCategories, catCounts)
	} else if args[0] == "Back" {
		if numArgs != 2 {
			usage(os.Args[0])
		}
		processSequence(false, args[1], client, flags, categoryMap, allCategories, catCounts)
	} else if args[0] == "CanonS100" {
		if numArgs != 1 {
			usage(os.Args[0])
		}
		canons100.ProcessCategory(canons100.CatInfo{"Canon PowerShot S100", "Category:Taken with unidentified Canon PowerShot S100", "Category:Taken with Canon PowerShot S100", "Category:Taken with Canon Digital IXUS"}, client, flags.verbose)

		canons100.ProcessCategory(canons100.CatInfo{"Canon PowerShot S110", "Category:Taken with unidentified Canon PowerShot S110", "Category:Taken with Canon PowerShot S110", "Category:Taken with Canon Digital IXUS v"}, client, flags.verbose)
	} else {
		usage(os.Args[0])
	}
}
