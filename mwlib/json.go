package mwlib

// Write an array of titles into a piped request string.
func MakeTitleString(titles []string) string {
	result := make([]byte, 0, 500)
	for i := range titles {
		if i > 0 {
			result = append(result, '|')
		}
		result = append(result, titles[i]...)
	}
	return string(result)
}
