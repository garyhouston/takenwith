package main

import (
	"fmt"
)

// Processing statistics.
type stats struct {
	examined   int32 // Total number of files examined.
	withCamera int32 // Number of files with camera details in Exif.
	errors     int32 // Number of files where an error was printed to output
	inCat      int32 // Number of files already in a relevant category.
	populated  int32 // Number of files skipped because of catFileLimit.
	edited     int32 // Number of files with attempt to edit.
}

func (s stats) print() {
	fmt.Println("Total files examined: ", s.examined)
	fmt.Println("Files with camera details in Exif: ", s.withCamera)
	fmt.Println("Files with errors printed: ", s.errors)
	fmt.Println("Files already categorised: ", s.inCat)
	fmt.Println("Files skipped due to CatFileLimit: ", s.populated)
	fmt.Println("Files attempted to edit: ", s.edited)
}
