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
	"strconv"
	"strings"
)

func addCategory(page string, category string, client *mwclient.Client) {
	// There's a small chance that saving a page may fail due to
	// an edit conflict or other transient error. Try up to 3
	// times before giving up.
	var saveError error
	for i := 0; i < 3; i++ {
		text, timestamp, err := client.GetPageByName(page)
		if err != nil {
			panic(fmt.Sprintf("%v %v", page, err))
		}
		// Add the category at the end of the text, since categories
		// are supposed to be at the end anyway. A previous version
		// tried to add after the last existing category, but that
		// can fail when the text contains comments.
		last := len(text)
		text = text[0:last] + "\n[[" + category + "]]" + text[last:]
		panic(text)
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
		panic(fmt.Sprintf("Failed to save %v %v", page, saveError))
	}
}

type fileTarget struct {
	title    string
	category string
}

func addCategories(pages []fileTarget, client *mwclient.Client, verbose bool, catFileLimit int32, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	for i := range pages {
		// The cat size limit needs to be checked again, since adding
		// previous files in the batch may have pushed it over the
		// limit.
		if catFileLimit > 0 && catCounts[pages[i].category] >= catFileLimit {
			stats.populated++
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
				stats.warnings++
			} else if verbose {
				fmt.Println(pages[i].title)
				fmt.Println("Adding to", pages[i].category, " (", catCounts[pages[i].category], " files)")
			}
			stats.edited++
			addCategory(pages[i].title, pages[i].category, client)
			if catFileLimit > 0 {
				incCatCount(pages[i].category, catCounts)
			}
		}
	}
}

// Remove files where the category already has more than catFileLimt members.
func filterCatLimit(cats []fileTarget, client *mwclient.Client, verbose bool, catFileLimit int32, catCounts map[string]int32, stats *stats) []fileTarget {
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
			stats.warnings++
			continue
		}
		if count >= catFileLimit {
			stats.populated++
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

func filterFiles(pages []exifcamera.FileCamera, client *mwclient.Client, verbose bool, categoryMap map[string]string, stats *stats) []fileTarget {
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
		stats.withCamera++
		var catMapped string
		catMapped, ok := categoryMap[pages[i].Make+pages[i].Model]
		if !ok {
			fmt.Println(pages[i].Title)
			fmt.Printf("No category for %v,%v\n", pages[i].Make, pages[i].Model)
			stats.warnings++
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
func filterCategories(files []fileTarget, client *mwclient.Client, verbose bool, ignoreCurrentCats bool, allCategories map[string]bool, stats *stats) []fileTarget {
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
				stats.inCat++
				if verbose {
					fmt.Println(files[i].title)
					fmt.Println("Already in mapped:", files[i].category)
				}
				found = true
				break
			}
			if !ignoreCurrentCats {
				_, found = allCategories[cats[j]]
				if found {
					stats.inCat++
					if verbose {
						fmt.Println(files[i].title)
						fmt.Println("Already in known:", cats[j])
					}
					break
				}
				if strings.HasPrefix(cats[j], "Category:Taken ") || strings.HasPrefix(cats[j], "Category:Scanned ") {
					stats.warnings++
					fmt.Println(files[i].title)
					fmt.Println("Already in unknown:", cats[j])
					found = true
					break
				}
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

func processFiles(fileArray []exifcamera.FileCamera, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	selected := filterFiles(fileArray, client, flags.verbose, categoryMap, stats)
	if len(selected) == 0 {
		return
	}
	if flags.catFileLimit > 0 {
		selected = filterCatLimit(selected, client, flags.verbose, flags.catFileLimit, catCounts, stats)
		if len(selected) == 0 {
			return
		}
	}
	selected = filterCategories(selected, client, flags.verbose, flags.ignoreCurrentCats, allCategories, stats)
	if len(selected) == 0 {
		return
	}
	addCategories(selected, client, flags.verbose, flags.catFileLimit, allCategories, catCounts, stats)
}

func processGenerator(params params.Values, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	lastFileProcessed := ""
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
			for id, page := range pagesMap {
				if id == "-1" {
					// Empty result set.
					return
				}
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
					pageArray[idx] = exifcamera.FileCamera{Title: title}
				} else {
					make, model := exifcamera.ExtractCamera(imageinfo[0])
					pageArray[idx] = exifcamera.FileCamera{Title: title, Make: make, Model: model}
				}
				idx++
				stats.examined++
			}
			lastFileProcessed = pageArray[len(pageArray)-1].Title
			processFiles(pageArray, client, flags, categoryMap, allCategories, catCounts, stats)
		}
		if flags.fileLimit > 0 && stats.examined >= flags.fileLimit {
			break
		}
		if flags.warningLimit > 0 && stats.warnings >= flags.warningLimit {
			break
		}
	}
	if query.Err() != nil {
		fmt.Println("Last file processed: ", lastFileProcessed)
		msg := query.Err().Error()
		if strings.Contains(msg, "This result was truncated") {
			// This is a nuisance. Some files have enormous Exif
			// data and the batch may exceed a limit of
			// 12,582,912 bytes for a single request.
			// E.g., File:Congressional Record Volume 81 Part 1.pdf
			fmt.Println(msg)
			fmt.Println("The combined Exif data was too large when processing a batch of files. Try running from the time of the last file processed with a smaller -batchSize.")
		} else {
			panic(query.Err())
		}
	}
}

func backString(back bool) string {
	if back {
		return "descending"
	} else {
		return "ascending"
	}
}

func processUser(user string, ts timestamp, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	params := params.Values{
		"generator": "allimages",
		"gaiuser":   strings.TrimPrefix(user, "User:"),
		"gaisort":   "timestamp",
		"gaidir":    backString(flags.back),
		"gailimit":  strconv.Itoa(flags.batchSize),
		"prop":      "imageinfo",
		"iiprop":    "metadata",
	}
	if ts.valid {
		params["gaistart"] = ts.string
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts, stats)
}

func processCategory(category string, ts timestamp, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	// Sorting is by the last modification of the file page. Image upload
	// time would be preferable.
	params := params.Values{
		"generator":    "categorymembers",
		"gcmtitle":     category,
		"gcmnamespace": "6", // namespace 6 for files on Commons.
		"gcmsort":      "timestamp",
		"gcmdir":       backString(flags.back),
		"gcmlimit":     strconv.Itoa(flags.batchSize),
		"prop":         "imageinfo",
		"iiprop":       "metadata",
	}
	if ts.valid {
		params["gcmstart"] = ts.string
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts, stats)
}

func processRandom(client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
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
		processGenerator(params, client, flags, categoryMap, allCategories, catCounts, stats)
	}
}

func processAll(ts timestamp, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	var direction string
	if flags.back {
		direction = "descending"
	} else {
		direction = "ascending"
	}
	params := params.Values{
		"generator": "allimages",
		"gaisort":   "timestamp",
		"gaidir":    direction,
		"gaistart":  ts.string,
		"gailimit":  strconv.Itoa(flags.batchSize),
		"prop":      "imageinfo",
		"iiprop":    "metadata",
	}
	processGenerator(params, client, flags, categoryMap, allCategories, catCounts, stats)
}

func processOneFile(page string, client *mwclient.Client, flags flags, categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	pageArray := make([]string, 1)
	pageArray[0] = page
	camArray := exifcamera.GetCameraInfo(pageArray, client)
	processFiles(camArray, client, flags, categoryMap, allCategories, catCounts, stats)
}

type flags struct {
	verbose           bool
	catFileLimit      int32
	user              string
	batchSize         int
	ignoreCurrentCats bool
	back              bool
	fileLimit         int32
	warningLimit      int32
}

func parseFlags() flags {
	iniflags.SetConfigFile(mwlib.GetWorkingDir() + "/takenwith.conf")
	var flags flags
	flag.BoolVar(&flags.verbose, "verbose", false, "Print action for every file")
	var catFileLimit int
	flag.IntVar(&catFileLimit, "catFileLimit", 100, "Don't add to categories with at least this many files. No limit if zero.")
	flag.StringVar(&flags.user, "user", "nobody@example.com", "Operator's email address or Wiki user name.")
	flag.IntVar(&flags.batchSize, "batchSize", 100, "Number of files to process per server request.")
	flag.BoolVar(&flags.ignoreCurrentCats, "ignoreCurrentCats", false, "Add to mapped categories even if already in a relevant category.")
	flag.BoolVar(&flags.back, "back", false, "Process backwards in time, from newer files to older files.")
	var fileLimit int
	flag.IntVar(&fileLimit, "fileLimit", 10000, "Stop after examining at least this many files. No limit if zero.")
	var warningLimit int
	flag.IntVar(&warningLimit, "warningLimit", 100, "Stop after printing at least this many warnings. No limit if zero.")
	iniflags.Parse()
	flags.catFileLimit = int32(catFileLimit)
	flags.fileLimit = int32(fileLimit)
	flags.warningLimit = int32(warningLimit)
	return flags
}

func usage(progName string) {
	log.Fatal("Usage: \n", progName, " File:f\n", progName, " User:u [timestamp]\n", progName, " Category:c [timestamp]\n", progName, " Random\n", progName, " All timestamp\n", progName, " CanonS100\n", "-help: display options.")
}

// Handler for processing to be done when bot is terminating.
func EndProc(client *mwclient.Client, stats *stats) {
	// Cookies can change while the bot is running, so save the latest values for the next run.
	mwlib.WriteCookies(client.DumpCookies())

	if stats.examined > 1 {
		fmt.Println()
		stats.print()
	}
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

	// Processing statistics.
	var stats stats

	if flags.catFileLimit > 0 {
		// Remove category counts for categories that we may need to update,
		// since the count may have changed since the bot last ran.
		removeSmallCounts(catCounts, flags.catFileLimit)
	}

	defer EndProc(client, &stats)

	args := flag.Args()
	numArgs := len(args)
	if numArgs == 0 || numArgs > 2 {
		usage(os.Args[0])
	}
	if strings.HasPrefix(args[0], "File:") {
		if numArgs > 1 {
			usage(os.Args[0])
		}
		processOneFile(args[0], client, flags, categoryMap, allCategories, catCounts, &stats)
	} else if strings.HasPrefix(args[0], "User:") {
		var ts timestamp
		if numArgs > 2 {
			usage(os.Args[0])
			return
		}
		if numArgs == 2 {
			ts, err = newTimestamp(args[1], true)
		} else {
			ts, err = newTimestamp("", false)
		}
		if err == nil {
			processUser(args[0], ts, client, flags, categoryMap, allCategories, catCounts, &stats)
		} else {
			printBadTimestamp()
		}
	} else if strings.HasPrefix(args[0], "Category:") {
		if numArgs > 2 {
			usage(os.Args[0])
			return
		}
		var ts timestamp
		if numArgs == 2 {
			ts, err = newTimestamp(args[1], true)
		} else {
			ts, err = newTimestamp("", false)
		}
		if err == nil {
			processCategory(args[0], ts, client, flags, categoryMap, allCategories, catCounts, &stats)
		} else {
			printBadTimestamp()
		}
	} else if args[0] == "Random" {
		if numArgs > 1 {
			usage(os.Args[0])
		} else {
			processRandom(client, flags, categoryMap, allCategories, catCounts, &stats)
		}
	} else if args[0] == "All" {
		if numArgs != 2 {
			usage(os.Args[0])
			return
		}
		ts, err := newTimestamp(args[1], true)
		if err == nil {
			processAll(ts, client, flags, categoryMap, allCategories, catCounts, &stats)
		} else {
			printBadTimestamp()
		}
	} else if args[0] == "CanonS100" {
		if numArgs != 1 {
			usage(os.Args[0])
		}
		canons100.ProcessCategory(canons100.CatInfo{ExifModel: "Canon PowerShot S100", UnidCategory: "Category:Taken with unidentified Canon PowerShot S100", PowershotCategory: "Category:Taken with Canon PowerShot S100", IxusCategory: "Category:Taken with Canon Digital IXUS"}, client, flags.verbose)

		canons100.ProcessCategory(canons100.CatInfo{ExifModel: "Canon PowerShot S110", UnidCategory: "Category:Taken with unidentified Canon PowerShot S110", PowershotCategory: "Category:Taken with Canon PowerShot S110", IxusCategory: "Category:Taken with Canon Digital IXUS v"}, client, flags.verbose)
	} else {
		usage(os.Args[0])
	}
}
