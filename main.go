package main

import (
	"fmt"
	"os"
)

func main() {

	fmt.Println("Loading PDF")
	if len(os.Args) < 2 {
		fmt.Println("No PDF file specified")
		os.Exit(1)
	}
	filename := os.Args[1]
	fmt.Println("Processing file:", filename)

}
