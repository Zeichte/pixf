package imageHandling

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chai2010/webp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// Buffer pool for encoding buffers to reduce allocations
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// getBuffer gets a buffer from the pool
func getBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

// putBuffer returns a buffer to the pool
func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

// ImageEncoder interface for encoding images
type ImageEncoder interface {
	Encode(w io.Writer, img *image.RGBA) error
	Extension() string
}

// PNG encoder with transparency
type PNGEncoder struct{}

func (e PNGEncoder) Encode(w io.Writer, img *image.RGBA) error {
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	return enc.Encode(w, img)
}

func (e PNGEncoder) Extension() string { return ".png" }

// WebP encoder with transparency
type WebPEncoder struct{}

func (e WebPEncoder) Encode(w io.Writer, img *image.RGBA) error {
	return webp.Encode(w, img, &webp.Options{
		Lossless: true,
		Quality:  100,
	})
}

func (e WebPEncoder) Extension() string { return ".webp" }

// Encoder registry
var encoderRegistry = map[string]ImageEncoder{
	"png":  PNGEncoder{},
	"webp": WebPEncoder{},
}

// GetEncoder returns encoder for the given format
func GetEncoder(format string) (ImageEncoder, error) {
	format = strings.ToLower(format)
	encoder, ok := encoderRegistry[format]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return encoder, nil
}

// Convert image to RGBA for transparency support
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba
}

// LoadedImage represents an image loaded from disk
type LoadedImage struct {
	Img      *image.RGBA // Pre-converted to RGBA for transparency support
	FileHash string      // Store only hash instead of full file data for deduplication
}

// ExtractImagesFromFile extracts images from a PDF file
// For 'original' format, uses PDFCPU's ExtractImageFile for native format
// For 'png' and 'webp', converts images with transparency support
func ExtractImagesFromFile(filename string, imgDir string, format string) error {
	if err := os.Mkdir(imgDir, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	// For original format, use PDFCPU's native extraction with deduplication
	if format == "original" || format == "" {
		return extractImagesOriginal(filename, imgDir)
	}

	// For other formats (png, webp), use concurrent processing
	encoder, err := GetEncoder(format)
	if err != nil {
		return err
	}

	return extractImagesConcurrent(filename, imgDir, encoder)
}

// extractImagesOriginal uses PDFCPU's ExtractImageFile for native format with deduplication
func extractImagesOriginal(filename string, imgDir string) error {
	// Extract images to temp directory
	tempDir, err := os.MkdirTemp("", "pdfimg")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := api.ExtractImagesFile(filename, tempDir, nil, nil); err != nil {
		return fmt.Errorf("api.ExtractImagesFile: %w", err)
	}

	// Read and process extracted images
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("read temp dir: %w", err)
	}

	return processExtractedFilesSequential(files, tempDir, imgDir)
}

// extractImagesConcurrent extracts images using concurrent goroutines
func extractImagesConcurrent(filename string, imgDir string, encoder ImageEncoder) error {
	// Extract images to temp directory
	tempDir, err := os.MkdirTemp("", "pdfimg")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := api.ExtractImagesFile(filename, tempDir, nil, nil); err != nil {
		return fmt.Errorf("api.ExtractImagesFile: %w", err)
	}

	// Read all image files first
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("read temp dir: %w", err)
	}

	// Collect image data with file hash for deduplication (single read per file)
	loadedImages := make([]LoadedImage, 0, len(files))

	for _, f := range files {
		if !isImageFile(f.Name()) {
			continue
		}

		imgPath := filepath.Join(tempDir, f.Name())

		// Single read: read file once, use for both decoding and hashing
		fileData, err := os.ReadFile(imgPath)
		if err != nil {
			return fmt.Errorf("read file data: %w", err)
		}

		// Decode image and convert to RGBA immediately
		rawImg, _, err := image.Decode(bytes.NewReader(fileData))
		if err != nil {
			return fmt.Errorf("decode image: %w", err)
		}

		// Convert to RGBA once upfront for encoding efficiency
		rgba := toRGBA(rawImg)

		// Compute hash immediately, release fileData reference after hash
		hash := hashBytes(fileData)

		loadedImages = append(loadedImages, LoadedImage{
			Img:      rgba,
			FileHash: hash,
		})
	}

	if len(loadedImages) == 0 {
		return nil
	}

	// Deduplicate using pre-computed hashes
	seen := make(map[string]bool)
	dupCount := 0
	uniqueImages := make([]LoadedImage, 0, len(loadedImages))

	for _, li := range loadedImages {
		if seen[li.FileHash] {
			dupCount++
			continue
		}
		seen[li.FileHash] = true
		uniqueImages = append(uniqueImages, li)
	}

	if dupCount > 0 {
		fmt.Printf("skipped %d duplicate image(s)\n", dupCount)
	}

	// Process unique images concurrently
	return processImagesConcurrently(uniqueImages, imgDir, encoder)
}

// processImagesConcurrently processes images with concurrent encoding
func processImagesConcurrently(loadedImages []LoadedImage, imgDir string, encoder ImageEncoder) error {
	type WriteTask struct {
		Index int
		Img   *image.RGBA
	}

	// Create tasks
	tasks := make([]WriteTask, 0, len(loadedImages))
	for i, li := range loadedImages {
		tasks = append(tasks, WriteTask{
			Index: i,
			Img:   li.Img,
		})
	}

	// Get configurable worker count from environment variable or use default
	numWorkers := getWorkerCount()
	taskChan := make(chan WriteTask, len(tasks))
	resultChan := make(chan error, len(tasks))
	var wg sync.WaitGroup
	var failed atomic.Bool

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				// Skip processing if already failed
				if failed.Load() {
					return
				}
				err := writeImageFile(task.Img, encoder, imgDir, task.Index)
				if err != nil {
					failed.Store(true)
					resultChan <- err
				} else {
					resultChan <- nil
				}
			}
		}()
	}

	// Send all tasks to workers
	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect first error
	var sendErr error
	for err := range resultChan {
		if err != nil && sendErr == nil {
			sendErr = err
		}
	}

	return sendErr
}

// getWorkerCount returns the number of workers from environment variable or default
func getWorkerCount() int {
	if val := os.Getenv("PIXF_WORKERS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return 4 // default worker count
}

// processExtractedFilesSequential processes all files sequentially for original format
func processExtractedFilesSequential(files []os.DirEntry, tempDir string, imgDir string) error {
	seen := make(map[string]bool)
	var dupCount int
	uniqueCount := 0

	for _, f := range files {
		if f.IsDir() || !isImageFile(f.Name()) {
			continue
		}

		imgPath := filepath.Join(tempDir, f.Name())

		// Read file content for deduplication
		fileData, err := os.ReadFile(imgPath)
		if err != nil {
			return fmt.Errorf("read image: %w", err)
		}

		// Hash the raw file content for deduplication
		hash := hashBytes(fileData)

		if seen[hash] {
			dupCount++
			continue
		}
		seen[hash] = true

		// Copy the file directly preserving original extension
		origExt := strings.ToLower(filepath.Ext(f.Name()))
		if origExt == "" {
			origExt = ".png"
		}
		dstPath := filepath.Join(imgDir, fmt.Sprintf("image_%04d%s", uniqueCount, origExt))
		if err := os.WriteFile(dstPath, fileData, 0644); err != nil {
			return fmt.Errorf("write image: %w", err)
		}
		uniqueCount++
	}

	if dupCount > 0 {
		fmt.Printf("skipped %d duplicate image(s)\n", dupCount)
	}

	return nil
}

// isImageFile checks if a filename has an image extension
func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".bmp" || ext == ".tiff" || ext == ".webp"
}

// hashBytes computes SHA-256 hash of byte slice
func hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// writeImageFile writes an encoded image to disk using buffer pooling
func writeImageFile(img *image.RGBA, encoder ImageEncoder, imgDir string, index int) error {
	buf := getBuffer()
	defer putBuffer(buf)

	if err := encoder.Encode(buf, img); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	ext := encoder.Extension()
	outName := fmt.Sprintf("image_%04d%s", index+1, ext)
	outPath := filepath.Join(imgDir, outName)

	return os.WriteFile(outPath, buf.Bytes(), 0644)
}
