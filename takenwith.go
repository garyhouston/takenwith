package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"fmt"
	"github.com/garyhouston/takenwith/canons100"
	"github.com/garyhouston/takenwith/exifcamera"
	mwlib "github.com/garyhouston/takenwith/mwlib"
	goflags "github.com/jessevdk/go-flags"
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
		text = text[0:last] + "\n[[" + category + "]]"
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

func addCategories(pages []fileTarget, client *mwclient.Client, verbose func(...string), catFileLimit int32, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	for i := range pages {
		// The cat size limit needs to be checked again, since adding
		// previous files in the batch may have pushed it over the
		// limit.
		if catFileLimit > 0 && catCounts[pages[i].category] >= catFileLimit {
			stats.populated++
			verbose(pages[i].title, "\n", "Already populated: ", pages[i].category)
		} else {
			// Identifying emtpy categories may help identify
			// when we are adding a file to a redirect page
			// for a renamed category. Ideally this would be done
			// regardless of catFileLimit, but counts aren't
			// maintained if it's zero.
			if catFileLimit > 0 && catCounts[pages[i].category] == 0 {
				warn(pages[i].title, "\n", "Adding to empty ", pages[i].category)
				stats.warnings++
			} else {
				verbose(pages[i].title, "\n", "Adding to ", pages[i].category, " (", strconv.Itoa(int(catCounts[pages[i].category])), " files)")
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
func filterCatLimit(cats []fileTarget, client *mwclient.Client, verbose func(...string), catFileLimit int32, catCounts map[string]int32, stats *stats) []fileTarget {
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
			warn(cats[i].title, "\n", "Mapped category doesn't exist: ", cats[i].category)
			stats.warnings++
			continue
		}
		if count >= catFileLimit {
			stats.populated++
			verbose(cats[i].title, "\n", "Already populated: ", cats[i].category)
			continue
		}
		result = result[0 : resultIdx+1]
		result[resultIdx] = cats[i]
		resultIdx++
	}
	return result
}

func filterFiles(pages []exifcamera.FileCamera, client *mwclient.Client, verbose func(...string), categoryMap map[string]string, stats *stats) []fileTarget {
	count := 0
	result := make([]fileTarget, 0, len(pages))
	for i := range pages {
		if pages[i].Make == "" && pages[i].Model == "" {
			verbose(pages[i].Title, "\n", "No camera details in Exif")
			continue
		}
		stats.withCamera++
		var catMapped string
		catMapped, ok := categoryMap[pages[i].Make+pages[i].Model]
		if !ok {
			warn(pages[i].Title, "\n", fmt.Sprintf("No category for %v,%v", pages[i].Make, pages[i].Model))
			stats.warnings++
			continue
		}
		if strings.HasPrefix(catMapped, "skip ") {
			verbose(pages[i].Title, "\n", "Skipping ", pages[i].Make, pages[i].Model)
			continue
		}
		result = result[0 : count+1]
		result[count] = fileTarget{pages[i].Title, catMapped}
		count++
	}
	return result
}

// Remove files which are already in a relevant category.
func filterCategories(files []fileTarget, client *mwclient.Client, verbose func(...string), ignoreCurrentCats bool, allCategories map[string]bool, stats *stats) []fileTarget {
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
				verbose(files[i].title, "\n", "Already in mapped: ", files[i].category)
				found = true
				break
			}
			if !ignoreCurrentCats {
				_, found = allCategories[cats[j]]
				if found {
					stats.inCat++
					verbose(files[i].title, "\n", "Already in known: ", cats[j])
					break
				}
				if strings.HasPrefix(cats[j], "Category:Taken ") || strings.HasPrefix(cats[j], "Category:Scanned ") {
					warn(files[i].title, "\n", "Already in unknown: ", cats[j])
					stats.warnings++
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

func processFiles(fileArray []exifcamera.FileCamera, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	selected := filterFiles(fileArray, client, verbose, categoryMap, stats)
	if len(selected) == 0 {
		return
	}
	if flags.CatFileLimit > 0 {
		selected = filterCatLimit(selected, client, verbose, flags.CatFileLimit, catCounts, stats)
		if len(selected) == 0 {
			return
		}
	}
	selected = filterCategories(selected, client, verbose, flags.IgnoreCurrentCats, allCategories, stats)
	if len(selected) == 0 {
		return
	}
	addCategories(selected, client, verbose, flags.CatFileLimit, allCategories, catCounts, stats)
}

func processGenerator(params params.Values, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	lastFileProcessed := ""
	query := client.NewQuery(params)
	for query.Next() {
		json := query.Resp()
		pages, err := json.GetObject("query", "pages")
		if err != nil {
			// result set may be empty due to "miser mode" in the
			// the Mediawiki server.
			continue
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
			processFiles(pageArray, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
		}
		if flags.FileLimit > 0 && stats.examined >= flags.FileLimit {
			break
		}
		if flags.WarningLimit > 0 && stats.warnings >= flags.WarningLimit {
			break
		}
	}
	if query.Err() != nil {
		fmt.Println("Last file processed: ", lastFileProcessed)
		msg := query.Err().Error()
		if strings.Contains(msg, "This result was truncated") {
			// This shouldn't happen now that we request
			// commonmetadata instead of metadata.
			// pdf / djvu files may contain complete text in
			// metadata, and a batch may exceed a limit of
			// 12,582,912 bytes for a single request.
			// E.g., File:Congressional Record Volume 81 Part 1.pdf
			// and its related files or
			// File:Boiste_-_Dictionnaire_universel,_1851.djvu
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

func processUser(user string, ts timestamp, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	params := params.Values{
		"generator": "allimages",
		"gaiuser":   strings.TrimPrefix(user, "User:"),
		"gaisort":   "timestamp",
		"gaidir":    backString(flags.Back),
		"gailimit":  strconv.Itoa(flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	if ts.valid {
		params["gaistart"] = ts.string
	}
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
}

func processCategory(category string, ts timestamp, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	// Sorting is by the last modification of the file page. Image upload
	// time would be preferable.
	params := params.Values{
		"generator":    "categorymembers",
		"gcmtitle":     category,
		"gcmnamespace": "6", // namespace 6 for files on Commons.
		"gcmsort":      "timestamp",
		"gcmdir":       backString(flags.Back),
		"gcmlimit":     strconv.Itoa(flags.BatchSize),
		"prop":         "imageinfo",
		"iiprop":       "commonmetadata",
	}
	if ts.valid {
		params["gcmstart"] = ts.string
	}
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
}

func processRandom(client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	batchSize := 20 // max accepted by random API.
	if flags.BatchSize < 20 {
		batchSize = flags.BatchSize
	}
	for {
		params := params.Values{
			"generator":    "random",
			"grnnamespace": "6", // namespace 6 for files on Commons.
			"grnlimit":     strconv.Itoa(batchSize),
			"prop":         "imageinfo",
			"iiprop":       "commonmetadata",
		}
		processGenerator(params, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
	}
}

func processAll(ts timestamp, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	var direction string
	if flags.Back {
		direction = "descending"
	} else {
		direction = "ascending"
	}
	params := params.Values{
		"generator": "allimages",
		"gaisort":   "timestamp",
		"gaidir":    direction,
		"gaistart":  ts.string,
		"gailimit":  strconv.Itoa(flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
}

func processOneFile(page string, client *mwclient.Client, flags flags, verbose func(...string), categoryMap map[string]string, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	pageArray := make([]string, 1)
	pageArray[0] = page
	camArray := exifcamera.GetCameraInfo(pageArray, client)
	processFiles(camArray, client, flags, verbose, categoryMap, allCategories, catCounts, stats)
}

type flags struct {
	Verbose           bool   `short:"v" long:"verbose" env:"takenwith_verbose" description:"Print action for every file"`
	CatFileLimit      int32  `short:"c" long:"catfilelimit" env:"takenwith_catfilelimit" description:"Don't add to categories with at least this many files. No limit if zero" default:"100"`
	Operator          string `long:"operator" env:"takenwith_operator" description:"Operator's email address or Wiki user name"`
	BatchSize         int    `short:"s" long:"batchsize" env:"takenwith_batchsize" description:"Number of files to process per server request" default:"100"`
	IgnoreCurrentCats bool   `short:"i" long:"ignorecurrentcats" env:"takenwith_ignorecurrentcats" description:"Add to mapped categories even if already in a relevant category"`
	Back              bool   `short:"b" long:"back" env:"takenwith_back" description:"Process backwards in time, from newer files to older files"`
	FileLimit         int32  `short:"f" long:"filelimit" env:"takenwith_filelimit" description:"Stop after examining at least this many files. No limit if zero" default:"10000"`
	WarningLimit      int32  `short:"w" long:"warninglimit" env:"takenwith_warninglimit" description:"Stop after printing at least this many warnings. No limit if zero" default:"100"`
}

func parseFlags() ([]string, flags) {
	var flags flags
	parser := goflags.NewParser(&flags, goflags.HelpFlag)
	parser.Usage = "[OPTIONS] File:f | User:u [timestamp] | Category:c [timestamp] | Random | All timestamp | CanonS100"
	args, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
	return args, flags
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

func warn(msgs ...string) {
	for _, msg := range msgs {
		fmt.Print(msg)
	}
	fmt.Print("\n")
}

// Return the function to be used for displaying (or not displaying) verbose
// messages.
func get_verbose(verbose bool) func(...string) {
	if verbose {
		return warn
	} else {
		/* Noop function. */
		return func(msgs ...string) {}
	}
}

func main() {
	args, flags := parseFlags()
	if flags.Operator == "" {
		log.Fatal("Operator email / username not set.")
	}
	verbose := get_verbose(flags.Verbose)
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "takenwith "+flags.Operator)
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

	if flags.CatFileLimit > 0 {
		// Remove category counts for categories that we may need to update,
		// since the count may have changed since the bot last ran.
		removeSmallCounts(catCounts, flags.CatFileLimit)
	}

	defer EndProc(client, &stats)

	numArgs := len(args)
	if numArgs == 0 || numArgs > 2 {
		log.Fatal("Command [timestamp] expected.")
	}
	if strings.HasPrefix(args[0], "File:") {
		if numArgs > 1 {
			log.Fatal("Unexpected parameter.")
		}
		processOneFile(args[0], client, flags, verbose, categoryMap, allCategories, catCounts, &stats)
	} else if args[0] == "Random" {
		if numArgs > 1 {
			log.Fatal("Unexpected parameter.")
		}
		processRandom(client, flags, verbose, categoryMap, allCategories, catCounts, &stats)
	} else if args[0] == "CanonS100" {
		if numArgs != 1 {
			log.Fatal("Unexpected parameter.")
		}
		canons100.ProcessCategory(canons100.CatInfo{ExifModel: "Canon PowerShot S100", UnidCategory: "Category:Taken with unidentified Canon PowerShot S100", PowershotCategory: "Category:Taken with Canon PowerShot S100", IxusCategory: "Category:Taken with Canon Digital IXUS"}, client, verbose)

		canons100.ProcessCategory(canons100.CatInfo{ExifModel: "Canon PowerShot S110", UnidCategory: "Category:Taken with unidentified Canon PowerShot S110", PowershotCategory: "Category:Taken with Canon PowerShot S110", IxusCategory: "Category:Taken with Canon Digital IXUS v"}, client, verbose)
	} else {
		var ts timestamp
		if numArgs == 2 {
			ts, err = newTimestamp(args[1], true)
		} else {
			ts, err = newTimestamp("", false)
		}
		if err != nil {
			printBadTimestamp()
			os.Exit(1)
		}
		if strings.HasPrefix(args[0], "User:") {
			processUser(args[0], ts, client, flags, verbose, categoryMap, allCategories, catCounts, &stats)
		} else if strings.HasPrefix(args[0], "Category:") {
			processCategory(args[0], ts, client, flags, verbose, categoryMap, allCategories, catCounts, &stats)
		} else if args[0] == "All" {
			if numArgs != 2 {
				log.Fatal("Timestamp required.")
			}
			processAll(ts, client, flags, verbose, categoryMap, allCategories, catCounts, &stats)
		} else {
			log.Fatal("Unknown command.")
		}
	}
}
