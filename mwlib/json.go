package mwlib

// Write an array of titles into a piped request string.
func MakeTitleString(titles []string) string {
	var result string = ""
	for i := range titles {
		if i > 0 {
			result += "|"
		}
		result += titles[i]
	}
	return result
}
