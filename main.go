package main

import (
	"fmt"
	"os"
	imageHandling "pixf/internal/toolset"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func main() {

	fmt.Println("Loading PDF")
	if len(os.Args) < 2 {
		fmt.Println("No PDF file specified")
		os.Exit(1)
	}
	filename := os.Args[1]
	filenameUnlocked := "unlocked_" + filename
	nameOnly := strings.TrimSuffix(filename, ".pdf")
	imgDir := "images_" + nameOnly
	fmt.Println("Processing file:", filename)

	// PDFCPU Unlocking
	conf := model.NewDefaultConfiguration()
	err := api.DecryptFile(filename, filenameUnlocked, conf)
	if err != nil {
		fmt.Println("Error decrypting PDF:", err)
		os.Exit(1)
	}
	fmt.Println("PDF successfully unlocked and saved as", filenameUnlocked)

	// PDFCPU Image Extraction
	err = imageHandling.ExtractImagesUnique(filenameUnlocked, imgDir)
	if err != nil {
		fmt.Println("Error extracting images:", err)
		os.Exit(1)
	}

}
