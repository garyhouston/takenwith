package main

import (
	mwclient "cgt.name/pkg/go-mwclient"
	"cgt.name/pkg/go-mwclient/params"
	"fmt"
	"github.com/antonholmquist/jason"
	"github.com/garyhouston/takenwith/mwlib"
	goflags "github.com/jessevdk/go-flags"
	"io/ioutil"
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

func incCatCount(category string, catCounts map[string]int32) {
	catCounts[category] = catCounts[category] + 1
}

func addCategories(files []fileData, client *mwclient.Client, verbose *log.Logger, catFileLimit int32, allCategories map[string]bool, catCounts map[string]int32, stats *stats) {
	for i := range files {
		if files[i].processed {
			continue
		}
		// The cat size limit needs to be checked again, since adding
		// previous files in the batch may have pushed it over the
		// limit.
		if catFileLimit > 0 && catCounts[files[i].catMapped] >= catFileLimit {
			stats.populated++
			verbose.Print(files[i].title, "\n", "Already populated: ", files[i].catMapped)
		} else {
			// Identifying emtpy categories helps identify
			// when we are adding a file to a redirect page
			// for a renamed category.
			if catCounts[files[i].catMapped] == 0 {
				warn.Print(files[i].title, "\n", "Adding to empty ", files[i].catMapped)
				files[i].warning = "Added to empty category"
				stats.warnings++
			} else {
				verbose.Printf("%s\nAdding to %s (%d files)", files[i].title, files[i].catMapped, int(catCounts[files[i].catMapped]))
			}
			stats.edited++
			addCategory(files[i].title, files[i].catMapped, client)
			incCatCount(files[i].catMapped, catCounts)
		}
		files[i].processed = true
	}
}

// For each file, cache the file count for its category if we don't already
// have it.
func cacheCatCounts(files []fileData, client *mwclient.Client, catCounts map[string]int32) {
	// Identify categories where the size isn't already cached. Use a map
	// to combine duplicates.
	lookup := make(map[string]bool)
	for i := range files {
		if !files[i].processed && files[i].catMapped != "" {
			_, found := catCounts[files[i].catMapped]
			if !found {
				lookup[files[i].catMapped] = true
			}
		}
	}
	// Try to cache the uncached
	if len(lookup) > 0 {
		cats := make([]string, len(lookup))
		idx := 0
		for key := range lookup {
			cats[idx] = key
			idx++
		}
		files, counts := catNumFiles(cats, client)
		for i := range files {
			catCounts[files[i]] = counts[i]
		}
	}
}

// Process files where the category is missing or already populated.
func filterCatLimit(files []fileData, client *mwclient.Client, verbose *log.Logger, catFileLimit int32, catCounts map[string]int32, stats *stats) {
	for i := range files {
		if !files[i].processed && files[i].catMapped != "" {
			count, found := catCounts[files[i].catMapped]
			if !found {
				warn.Print(files[i].title, "\n", "Mapped category doesn't exist: ", files[i].catMapped)
				files[i].warning = files[i].catMapped + " doesn't exist"
				stats.warnings++
				files[i].processed = true
				continue
			}
			if catFileLimit > 0 && count >= catFileLimit {
				stats.populated++
				verbose.Print(files[i].title, "\n", "Already populated: ", files[i].catMapped)
				files[i].processed = true
				continue
			}
		}
	}
}

// Determine if any of cats (a file's current categories) match either the
// Exif target category, any known target category, or any unknown category
// that's named like a target category.
func matchCategories(file *fileData, cats []string, mapped string, verbose *log.Logger, ignoreCurrentCats bool, allCategories map[string]bool, stats *stats) bool {
	result := false
	for _, cat := range cats {
		if mapped == cat {
			stats.inCat++
			verbose.Print(file.title, "\n", "Already in mapped: ", mapped)
			result = true
			break
		}
		if !ignoreCurrentCats {
			if allCategories[cat] {
				result = true
				stats.inCat++
				verbose.Print(file.title, "\n", "Already in known: ", cat)
				break
			}
			if strings.HasPrefix(cat, "Category:Taken ") || strings.HasPrefix(cat, "Category:Scanned ") {
				result = true
				warn.Print(file.title, "\n", "Already in unknown: ", cat)
				file.warning = "In unknown " + cat
				stats.warnings++
				break
			}
		}
	}
	return result
}

// Process files which are already in a relevant category.
func filterCategories(files []fileData, client *mwclient.Client, verbose *log.Logger, ignoreCurrentCats bool, allCategories map[string]bool, stats *stats) {
	titles := make([]string, len(files))
	idx := 0
	for i := range files {
		if !files[i].processed {
			titles[idx] = files[i].title
			idx++
		}
	}
	if idx == 0 {
		return
	}
	titles = titles[0:idx]
	fileCats := getPageCategories(titles, client)
	for i := range files {
		if files[i].processed {
			continue
		}
		cats := fileCats[files[i].title]
		if matchCategories(&files[i], cats, files[i].catMapped, verbose, ignoreCurrentCats, allCategories, stats) {
			files[i].processed = true
		} else {
			if files[i].catMapped == "" {
				// Handle the delayed error case from
				// mapCategories, now that we know it's not in
				// a relevant category.
				warn.Printf("%s\nNo category for %v,%v", files[i].title, files[i].make, files[i].model)
				files[i].warning = files[i].make + " " + files[i].model
				stats.warnings++
				files[i].processed = true
			}
		}
	}
}

func applyRegex(key string, catRegex []catRegex) string {
	for i := range catRegex {
		loc := catRegex[i].regex.FindStringIndex(key)
		if loc != nil {
			return catRegex[i].target
		}
	}
	return ""
}

// Determine Commons category from imageinfo (Exif) data, if possible.
func mapCategories(files []fileData, verbose *log.Logger, categoryMap map[string]string, catRegex []catRegex, stats *stats) {
	for i := range files {
		var err error
		files[i].title, err = files[i].pageObj.GetString("title")
		if err != nil {
			panic(err)
		}
		imageinfo, err := files[i].pageObj.GetObjectArray("imageinfo")
		if err == nil {
			files[i].make, files[i].model = extractCamera(imageinfo[0])
		}
		if err != nil || (files[i].make == "" && files[i].model == "") {
			verbose.Print(files[i].title, "\n", "No camera details in Exif")
			files[i].processed = true
			continue
		}
		stats.withCamera++
		// Category mapping: first try the simple map for an exact
		// match (which is fast), when try each regex match in turn.
		// If mapping fails, processing continues with blank catMapped
		// to determine file's current categories before displaying a
		// warning.
		key := files[i].make + files[i].model
		var found bool
		files[i].catMapped, found = categoryMap[key]
		if !found {
			files[i].catMapped = applyRegex(key, catRegex)
		}

		if files[i].catMapped == "Category:CanonS100 (special case)" {
			files[i].catMapped = mapCanonS100(imageinfo)
		} else if files[i].catMapped == "Category:CanonS110 (special case)" {
			files[i].catMapped = mapCanonS110(imageinfo)
		}
	}
}

func processFiles(files []fileData, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, catCounts map[string]int32, stats *stats) {
	mapCategories(files, verbose, categoryMap, catRegex, stats)
	cacheCatCounts(files, client, catCounts)
	filterCatLimit(files, client, verbose, flags.CatFileLimit, catCounts, stats)
	filterCategories(files, client, verbose, flags.IgnoreCurrentCats, allCategories, stats)
	addCategories(files, client, verbose, flags.CatFileLimit, allCategories, catCounts, stats)
}

// Data obtained about a single Wiki file page.
type fileData struct {
	pageObj   *jason.Object // Result of a query pages request that includes imageinfo.
	title     string        // Title of the Wiki file page.
	make      string        // Equipment make from Exif.
	model     string        // Equipment model from Exif.
	catMapped string        // Category name	mapped from Exif equipment make/model, or blank if the lookup fails.
	processed bool          // True once file has been fully processed.
	warning   string        // Brief warning string.
}

func checkWarnings(gallery string, warnings *warnings, client *mwclient.Client) {
	if len(*warnings) > 0 {
		warnings.createGallery(gallery, client)
	}
}

func processGenerator(params params.Values, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
	catCounts := make(map[string]int32)
	warnings := make(warnings, 0, 200)
	if flags.Gallery != "" {
		// try to write gallery even if there's a panic while processing files.
		defer checkWarnings(flags.Gallery, &warnings, client)
	}
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
			files := make([]fileData, len(pagesMap))
			idx := 0
			for id, page := range pagesMap {
				if id == "-1" {
					// Empty result set.
					return
				}
				files[idx].pageObj, err = page.Object()
				if err != nil {
					panic(err)
				}
				idx++
				stats.examined++
			}
			if idx > 0 {
				processFiles(files, client, flags, verbose, categoryMap, allCategories, catRegex, catCounts, stats)
				warnings.Append(files)
			}
		}
		if flags.FileLimit > 0 && stats.examined >= flags.FileLimit {
			break
		}
		if flags.WarningLimit > 0 && stats.warnings >= flags.WarningLimit {
			break
		}
	}
	if query.Err() != nil {
		panic(query.Err())
	}
}

func backString(back bool) string {
	if back {
		return "descending"
	} else {
		return "ascending"
	}
}

func processUser(user string, ts timestamp, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
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
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catRegex, stats)
}

func processCategory(category string, ts timestamp, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
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
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catRegex, stats)
}

func processRandom(client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
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
		processGenerator(params, client, flags, verbose, categoryMap, allCategories, catRegex, stats)
	}
}

// Process the images embedded in a page, e.g., a gallery.
func processPage(page string, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
	params := params.Values{
		"generator": "images",
		"titles":    page,
		"gimlimit":  strconv.Itoa(flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catRegex, stats)
}

func processAll(ts timestamp, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
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
	processGenerator(params, client, flags, verbose, categoryMap, allCategories, catRegex, stats)
}

// Return a json object containing page title and imageinfo (Exif) data.
func GetImageinfo(page string, client *mwclient.Client) *jason.Object {
	params := params.Values{
		"action":    "query",
		"titles":    page,
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
		"redirects": "", // follow redirects
		"continue":  "",
	}
	json, err := client.Get(params)
	if err != nil {
		panic(err)
	}
	return mwlib.GetJsonPage(json)
}

func processOneFile(page string, client *mwclient.Client, flags flags, verbose *log.Logger, categoryMap map[string]string, allCategories map[string]bool, catRegex []catRegex, stats *stats) {
	catCounts := make(map[string]int32)
	files := make([]fileData, 1)
	files[0].pageObj = GetImageinfo(page, client)
	if files[0].pageObj == nil {
		warn.Print(page, " does not exist, possibly deleted.")
		return
	}
	processFiles(files, client, flags, verbose, categoryMap, allCategories, catRegex, catCounts, stats)
}

type flags struct {
	Verbose           bool   `short:"v" long:"verbose" env:"takenwith_verbose" description:"Print action for every file"`
	CatFileLimit      int32  `short:"c" long:"catfilelimit" env:"takenwith_catfilelimit" description:"Don't add to categories with at least this many files. No limit if zero" default:"100"`
	Operator          string `long:"operator" env:"takenwith_operator" description:"Operator's email address or Wiki user name"`
	MappingFile       string `long:"mappingfile" env:"takenwith_mappingfile" description:"Path of the catmapping file"`
	ExceptionFile     string `long:"exceptionfile" env:"takenwith_exceptionfile" description:"Path of the catexceptions file"`
	RegexFile         string `long:"regexfile" env:"takenwith_regexfile" description:"Path of the category regex file"`
	CookieFile        string `long:"cookiefile" env:"takenwith_cookiefile" description:"Path of the cookies cache file"`
	BatchSize         int    `short:"s" long:"batchsize" env:"takenwith_batchsize" description:"Number of files to process per server request" default:"100"`
	IgnoreCurrentCats bool   `short:"i" long:"ignorecurrentcats" env:"takenwith_ignorecurrentcats" description:"Add to mapped categories even if already in a relevant category"`
	Back              bool   `short:"b" long:"back" env:"takenwith_back" description:"Process backwards in time, from newer files to older files"`
	FileLimit         int32  `short:"f" long:"filelimit" env:"takenwith_filelimit" description:"Stop after examining at least this many files. No limit if zero" default:"10000"`
	WarningLimit      int32  `short:"w" long:"warninglimit" env:"takenwith_warninglimit" description:"Stop after printing at least this many warnings. No limit if zero" default:"100"`
	Gallery           string `long:"gallery" env:"takenwith_gallery" description:"Gallery page in which to display files with warnings"`
}

func parseFlags() ([]string, flags) {
	var flags flags
	parser := goflags.NewParser(&flags, goflags.HelpFlag)
	parser.Usage = "[OPTIONS] File:f | User:u [timestamp] | Category:c [timestamp] | Random | Page:p | All timestamp"
	args, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
	return args, flags
}

// Handler for processing to be done when bot is terminating.
func EndProc(client *mwclient.Client, stats *stats, cookieFile string) {
	// Cookies can change while the bot is running, so save the latest values for the next run.
	mwlib.WriteCookies(client.DumpCookies(), cookieFile)

	if stats.examined > 1 {
		fmt.Println()
		stats.print()
	}
}

var warn = log.New(os.Stdout, "", 0)

// Return the logger to be used for displaying (or not displaying) verbose
// messages.
func get_verbose(verbose bool) *log.Logger {
	if verbose {
		return log.New(os.Stdout, "", 0)
	} else {
		/* Noop logger. */
		return log.New(ioutil.Discard, "", 0)
	}
}

// Return true if the client is logged in.
func checkLogin(client *mwclient.Client) bool {
	params := params.Values{
		"action":   "query",
		"assert":   "user",
		"continue": "",
	}
	_, err := client.Get(params)
	return err == nil
}

func clearCookies(client *mwclient.Client, cookieFile string) {
	cookies := mwlib.ReadCookies(cookieFile)
	for idx, _ := range cookies {
		cookies[idx].MaxAge = -1
	}
	client.LoadCookies(cookies)
}

func login(client *mwclient.Client, flags flags) bool {
	// Clear old session cookies, otherwise they remain in the cookiejar
	// as duplicates and remain in use.
	clearCookies(client, flags.CookieFile)
	username := os.Getenv("takenwith_username")
	if username == "" {
		warn.Print("Username for login not set in environment.")
		return false
	}
	password := os.Getenv("takenwith_password")
	if password == "" {
		warn.Print("Password for login not set in environment.")
		return false
	}
	err := client.Login(username, password)
	if err != nil {
		log.Print(err)
		return false
	}
	return true
}

func main() {
	args, flags := parseFlags()
	if flags.Operator == "" {
		warn.Print("Operator email / username not set.")
		return
	}
	if flags.MappingFile == "" {
		warn.Print("Category mapping file path not set.")
		return
	}
	if flags.ExceptionFile == "" {
		warn.Print("Category exception file path not set.")
		return
	}
	if flags.RegexFile == "" {
		warn.Print("Category regex file path not set.")
		return
	}
	if flags.CookieFile == "" {
		warn.Print("Cookie cache file path not set.")
		return
	}
	verbose := get_verbose(flags.Verbose)
	client, err := mwclient.New("https://commons.wikimedia.org/w/api.php", "takenwith "+flags.Operator)
	if err != nil {
		panic(err)
	}
	client.Maxlag.On = true

	cookies := mwlib.ReadCookies(flags.CookieFile)
	client.LoadCookies(cookies)

	categoryMap := fillCategoryMap(flags.MappingFile) // makemodel -> category

	// All known categories, including those that aren't catmapping
	// targets.
	allCategories := fillCategories(categoryMap, flags.ExceptionFile)

	catRegex := fillRegex(flags.RegexFile)

	var stats stats

	defer EndProc(client, &stats, flags.CookieFile)

	if !checkLogin(client) {
		if !login(client, flags) {
			return
		}
	}

	numArgs := len(args)
	if numArgs == 0 || numArgs > 2 {
		warn.Print("Command [timestamp] expected.")
		return
	}
	if strings.HasPrefix(args[0], "File:") {
		if numArgs > 1 {
			warn.Print("Unexpected parameter.")
			return
		}
		processOneFile(args[0], client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
	} else if args[0] == "Random" {
		if numArgs > 1 {
			warn.Print("Unexpected parameter.")
			return
		}
		processRandom(client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
	} else if strings.HasPrefix(args[0], "Page:") {
		if numArgs > 1 {
			warn.Print("Unexpected parameter.")
			return
		}
		processPage(args[0][5:], client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
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
			processUser(args[0], ts, client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
		} else if strings.HasPrefix(args[0], "Category:") {
			processCategory(args[0], ts, client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
		} else if args[0] == "All" {
			if numArgs != 2 {
				warn.Print("Timestamp required.")
				return
			}
			processAll(ts, client, flags, verbose, categoryMap, allCategories, catRegex, &stats)
		} else {
			warn.Print("Unknown command.")
			return
		}
	}
}
