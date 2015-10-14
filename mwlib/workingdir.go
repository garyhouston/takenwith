package mwlib

import (
	"os"
)

func GetWorkingDir() string {
	dir := os.Getenv("WIKI_BOTTING_DIR")
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		dir = wd
	}
	return dir
}
