package imageHandling

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func ExtractImagesUnique(filename string, imgDir string) error {

	// CREATION OF A NEW DIRECTORY FOR IMAGES
	err := os.Mkdir(imgDir, 0755)
	if err != nil {
		return err
	}

	// EXTRACTIONS OF IMAGES
	err = api.ExtractImagesFile(filename, imgDir, nil, nil)
	if err != nil {
		return err
	}

	// KEEP ONLY UNIQUE IMAGES
	var countDuplicates int
	imgHashes := make(map[string]bool)
	files, err := os.ReadDir(imgDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(imgDir, f.Name())

		// Calculate Hash
		file, _ := os.Open(path)
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		_ = file.Close()
		if err != nil {
			return err
		}

		fileHash := fmt.Sprintf("%x", hash.Sum(nil))

		if !imgHashes[fileHash] {
			imgHashes[fileHash] = true
		} else {
			err = os.Remove(path)
			if err != nil {
				return err
			}
			countDuplicates++
		}
	}

	return nil
}
