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
	"regexp"
	"strconv"
	"strings"
	"time"
)

// "global" exectution state.
type state struct {
	client        *mwclient.Client
	flags         flags
	verbose       log.Logger
	categoryMap   map[string]string
	allCategories map[string]bool
	catRegex      []catRegex
	stats         stats
	lastEdit      time.Time
}

// insertPos finds a position in a page to insert a category: a) after
// the last existing category, ignoring categories in unparsed
// sections (HTML comments, <pre> etc.) b) before an unterminated
// unparsed section c) at the end of the page.
func insertPos(page string) int {
	// Assume that unparsed sections don't nest, but don't assume
	// that a matching end tag is present.
	page = strings.ToLower(page) // Ignore case when matching tags and category.
	startTags := []string{"<!--", "<nowiki>", "<pre>", "<math>"}
	endTags := []string{"-->", "</nowiki>", "</pre>", "</math>"}
	start := -1
	startTag := ""
	endTag := ""
	// Find the first non-parsed section, if any.
	for i := 0; i < len(startTags); i++ {
		pos := strings.Index(page, startTags[i])
		if pos >= 0 && (start == -1 || pos < start) {
			start = pos
			startTag = startTags[i]
			endTag = endTags[i]
		}

	}
	unterminated := false
	if start >= 0 {
		startTagLen := len(startTag)
		end := strings.Index(page[start+startTagLen:], endTag)
		if end == -1 {
			end = len(page)
			unterminated = true
		} else {
			end += start + startTagLen + len(endTag)
		}
		page = page[:start] + strings.Repeat(" ", end-start) + page[end:]
		if !unterminated {
			insertPos(page)
		}
	}
	regexp := regexp.MustCompile("\\[\\[category\\:[^]]*\\]\\]")
	matches := regexp.FindAllIndex([]byte(page), -1)
	if len(matches) > 0 {
		return matches[len(matches)-1][1] // end position of the last match.
	}
	if unterminated {
		return start
	}
	return len(page)
}

func addCategory(page string, category string, remove string, client *mwclient.Client, lastEdit *time.Time) error {
	// Don't attempt to edit more than once per 5 seconds, per Commons bot policy
	dur := time.Since(*lastEdit)
	if dur.Seconds() < 5 {
		time.Sleep(time.Duration(5)*time.Second - dur)
	}
	// There's a small chance that saving a page may fail due to
	// an edit conflict or other transient error. Try up to 3
	// times before giving up.
	var saveError error
	for i := 0; i < 3; i++ {
		text, timestamp, err := client.GetPageByName(page)
		if err != nil {
			fmt.Println(timestamp)
			fmt.Println(text)
			panic(fmt.Sprintf("%v %v", page, err))
		}
		summary := ""
		if remove != "" {
			// Remove a category.
			regexp := regexp.MustCompile("\\n?\\[\\[[Cc]ategory\\:" + remove + "\\]\\]")
			text = string(regexp.ReplaceAll([]byte(text), []byte("")))
			summary = "moved from [[Category:" + remove + "]] to [[" + category + "]]"
		} else {
			summary = "added [[" + category + "]]"
		}
		pos := insertPos(text)
		text = text[0:pos] + "\n[[" + category + "]]" + text[pos:]
		editcfg := map[string]string{
			"action":        "edit",
			"title":         page,
			"text":          text,
			"summary":       summary,
			"minor":         "",
			"bot":           "",
			"basetimestamp": timestamp,
		}
		saveError = client.Edit(editcfg)
		if saveError == nil {
			break
		}
		if strings.Contains(saveError.Error(), "protected") {
			return saveError
		}
	}
	if saveError != nil {
		panic(fmt.Sprintf("Failed to save %v %v", page, saveError))
	}
	*lastEdit = time.Now()
	return nil
}

func incCatCount(category string, catCounts map[string]int32) {
	catCounts[category] = catCounts[category] + 1
}

func addCategories(files []fileData, catCounts map[string]int32, state *state) {
	for i := range files {
		if files[i].processed {
			continue
		}
		// The cat size limit needs to be checked again, since adding
		// previous files in the batch may have pushed it over the
		// limit.
		if state.flags.CatFileLimit > 0 && catCounts[files[i].catMapped] >= state.flags.CatFileLimit {
			state.stats.populated++
			state.verbose.Print(files[i].title, "\n", "Already populated: ", files[i].catMapped)
		} else {
			// Identifying emtpy categories helps identify
			// when we are adding a file to a redirect page
			// for a renamed category.
			if catCounts[files[i].catMapped] == 0 {
				warn.Print(files[i].title, "\n", "Adding to empty ", files[i].catMapped)
				files[i].warning = "Added to empty category"
				state.stats.warnings++
			} else {
				state.verbose.Printf("%s\nAdding to %s (%d files)", files[i].title, files[i].catMapped, int(catCounts[files[i].catMapped]))
			}
			err := addCategory(files[i].title, files[i].catMapped, state.flags.Remove, state.client, &state.lastEdit)
			if err == nil {
				state.stats.edited++
				incCatCount(files[i].catMapped, catCounts)
			} else {
				warn.Print(files[i].title, "\n", err.Error(), "\n")
				files[i].warning = err.Error()
				state.stats.warnings++
			}
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
			if strings.HasPrefix(cat, "Category:Taken with ") || strings.HasPrefix(cat, "Category:Scanned with ") {
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
func mapCategories(files []fileData, state *state) {
	for i := range files {
		var err error
		files[i].title, err = files[i].pageObj.GetString("title")
		if err != nil {
			panic(err)
		}
		missing, err := files[i].pageObj.GetBoolean("missing")
		if err == nil && missing {
			warn.Print(files[i].title, "\n", "File not found; may have been deleted.\n")
			files[i].warning = "File not found"
			state.stats.warnings++
			files[i].processed = true
			continue
		}
		imageinfo, err := files[i].pageObj.GetObjectArray("imageinfo")
		if err == nil {
			files[i].make, files[i].model = extractCamera(imageinfo[0])
		}
		if err != nil || (files[i].make == "" && files[i].model == "") {
			state.verbose.Print(files[i].title, "\n", "No camera details in Exif")
			files[i].processed = true
			continue
		}
		state.stats.withCamera++
		// Category mapping: first try the simple map for an exact
		// match (which is fast), when try each regex match in turn.
		// If mapping fails, processing continues with blank catMapped
		// to determine file's current categories before displaying a
		// warning.
		key := files[i].make + files[i].model
		var found bool
		files[i].catMapped, found = state.categoryMap[key]
		if !found {
			files[i].catMapped = applyRegex(key, state.catRegex)
		}

		if files[i].catMapped == "Category:CanonS100 (special case)" {
			files[i].catMapped = mapCanonS100(imageinfo)
		} else if files[i].catMapped == "Category:CanonS110 (special case)" {
			files[i].catMapped = mapCanonS110(imageinfo)
		}
	}
}

func processFiles(files []fileData, catCounts map[string]int32, state *state) {
	mapCategories(files, state)
	cacheCatCounts(files, state.client, catCounts)
	filterCatLimit(files, state.client, &state.verbose, state.flags.CatFileLimit, catCounts, &state.stats)
	filterCategories(files, state.client, &state.verbose, state.flags.IgnoreCurrentCats, state.allCategories, &state.stats)
	addCategories(files, catCounts, state)
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

func processGenerator(params params.Values, state *state) {
	catCounts := make(map[string]int32)
	warnings := make(warnings, 0, 200)
	if state.flags.Gallery != "" {
		// try to write gallery even if there's a panic while processing files.
		defer checkWarnings(state.flags.Gallery, &warnings, state.client)
	}
	query := state.client.NewQuery(params)
	for query.Next() {
		json := query.Resp()
		pages, err := json.GetObjectArray("query", "pages")
		if err != nil {
			// This happens if the query returns a continuation but no pages, e.g., when processing a category with a lot of subcategories, e.g., Category:Photos taken with Samsung mobile phones
			continue
		}
		if len(pages) == 0 {
			break
		} else {
			files := make([]fileData, len(pages))
			for i, _ := range pages {
				files[i].pageObj, err = pages[i].Object()
				if err != nil {
					panic(err)
				}
				state.stats.examined++
			}
			processFiles(files, catCounts, state)
			warnings.Append(files)
		}
		if state.flags.FileLimit > 0 && state.stats.examined >= state.flags.FileLimit {
			break
		}
		if state.flags.WarningLimit > 0 && state.stats.warnings >= state.flags.WarningLimit {
			break
		}
	}
	if query.Err() != nil {
		// A cryptic error here like "invalid character '<' looking for beginning of value" can typically be avoided by
		// reducing the batch size (-s option).
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

func processUser(user string, ts timestamp, state *state) {
	params := params.Values{
		"generator": "allimages",
		"gaiuser":   strings.TrimPrefix(user, "User:"),
		"gaisort":   "timestamp",
		"gaidir":    backString(state.flags.Back),
		"gailimit":  strconv.Itoa(state.flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	if ts.valid {
		params["gaistart"] = ts.string
	}
	processGenerator(params, state)
}

func processCategory(category string, ts timestamp, state *state) {
	// Sorting is by the last modification of the file page. Image upload
	// time would be preferable.
	params := params.Values{
		"generator":    "categorymembers",
		"gcmtitle":     category,
		"gcmnamespace": "6", // namespace 6 for files on Commons.
		"gcmsort":      "timestamp",
		"gcmdir":       backString(state.flags.Back),
		"gcmlimit":     strconv.Itoa(state.flags.BatchSize),
		"prop":         "imageinfo",
		"iiprop":       "commonmetadata",
	}
	if ts.valid {
		params["gcmstart"] = ts.string
	}
	processGenerator(params, state)
}

func processRandom(state *state) {
	batchSize := 20 // max accepted by random API.
	if state.flags.BatchSize < 20 {
		batchSize = state.flags.BatchSize
	}
	for {
		params := params.Values{
			"generator":    "random",
			"grnnamespace": "6", // namespace 6 for files on Commons.
			"grnlimit":     strconv.Itoa(batchSize),
			"prop":         "imageinfo",
			"iiprop":       "commonmetadata",
		}
		processGenerator(params, state)
	}
}

// Process the images embedded in a page, e.g., a gallery.
func processPage(page string, state *state) {
	params := params.Values{
		"generator": "images",
		"titles":    page,
		"gimlimit":  strconv.Itoa(state.flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	processGenerator(params, state)
}

func processAll(ts timestamp, state *state) {
	var direction string
	if state.flags.Back {
		direction = "descending"
	} else {
		direction = "ascending"
	}
	params := params.Values{
		"generator": "allimages",
		"gaisort":   "timestamp",
		"gaidir":    direction,
		"gaistart":  ts.string,
		"gailimit":  strconv.Itoa(state.flags.BatchSize),
		"prop":      "imageinfo",
		"iiprop":    "commonmetadata",
	}
	processGenerator(params, state)
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
	pages, err := json.GetObjectArray("query", "pages")
	if err != nil {
		fmt.Println(json)
		panic(err)
	}
	obj, err := pages[0].Object()
	if err != nil {
		fmt.Println(json)
		panic(err)
	}
	missing, err := obj.GetBoolean("missing")
	if err == nil && missing {
		return nil
	}
	return obj
}

func processOneFile(page string, state *state) {
	catCounts := make(map[string]int32)
	files := make([]fileData, 1)
	files[0].pageObj = GetImageinfo(page, state.client)
	if files[0].pageObj == nil {
		warn.Print(page, " does not exist, possibly deleted.")
		return
	}
	processFiles(files, catCounts, state)
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
	Remove            string `short:"r" long:"remove" env:"takenwith_remove" description:"When adding a category, remove this category. Do not include a Category: prefix."`
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
	var state state
	var args []string
	args, state.flags = parseFlags()
	if state.flags.Operator == "" {
		warn.Print("Operator email / username not set.")
		return
	}
	if state.flags.MappingFile == "" {
		warn.Print("Category mapping file path not set.")
		return
	}
	if state.flags.ExceptionFile == "" {
		warn.Print("Category exception file path not set.")
		return
	}
	if state.flags.RegexFile == "" {
		warn.Print("Category regex file path not set.")
		return
	}
	if state.flags.CookieFile == "" {
		warn.Print("Cookie cache file path not set.")
		return
	}
	state.verbose = *get_verbose(state.flags.Verbose)
	var err error
	state.client, err = mwclient.New("https://commons.wikimedia.org/w/api.php", "takenwith "+state.flags.Operator)
	if err != nil {
		panic(err)
	}
	state.client.Maxlag.On = true

	cookies := mwlib.ReadCookies(state.flags.CookieFile)
	state.client.LoadCookies(cookies)

	state.categoryMap = fillCategoryMap(state.flags.MappingFile) // makemodel -> category

	// All known categories, including those that aren't catmapping
	// targets.
	state.allCategories = fillCategories(state.categoryMap, state.flags.ExceptionFile)

	state.catRegex = fillRegex(state.flags.RegexFile)

	defer EndProc(state.client, &state.stats, state.flags.CookieFile)

	if !checkLogin(state.client) {
		if !login(state.client, state.flags) {
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
		processOneFile(args[0], &state)
	} else if args[0] == "Random" {
		if numArgs > 1 {
			warn.Print("Unexpected parameter.")
			return
		}
		processRandom(&state)
	} else if strings.HasPrefix(args[0], "Page:") {
		if numArgs > 1 {
			warn.Print("Unexpected parameter.")
			return
		}
		processPage(args[0][5:], &state)
	} else {
		var ts timestamp
		if numArgs == 2 {
			ts, err = newTimestamp(args[1])
			if err != nil {
				warn.Print(err)
				return
			}
		} else {
			ts = newTimestampEmpty()
		}
		if strings.HasPrefix(args[0], "User:") {
			processUser(args[0], ts, &state)
		} else if strings.HasPrefix(args[0], "Category:") {
			processCategory(args[0], ts, &state)
		} else if args[0] == "All" {
			if numArgs != 2 {
				warn.Print("Timestamp required.")
				return
			}
			processAll(ts, &state)
		} else {
			warn.Print("Unknown command.")
			return
		}
	}
}
