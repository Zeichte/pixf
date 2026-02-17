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
	"runtime"
	"strings"
	"sync"

	"github.com/chai2010/webp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// Buffer pool for encoding to reduce allocations
var bufferPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

func getBuffer() *bytes.Buffer { return bufferPool.Get().(*bytes.Buffer) }
func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

// ImageEncoder interface
type ImageEncoder interface {
	Encode(w io.Writer, img *image.RGBA) error
	Extension() string
}

// Encoders
type PNGEncoder struct{}

func (PNGEncoder) Encode(w io.Writer, img *image.RGBA) error {
	return png.Encoder{CompressionLevel: png.NoCompression}.Encode(w, img)
}
func (PNGEncoder) Extension() string { return ".png" }

type WebPEncoder struct{}

func (WebPEncoder) Encode(w io.Writer, img *image.RGBA) error {
	return webp.Encode(w, img, &webp.Options{Lossless: true, Quality: 100})
}
func (WebPEncoder) Extension() string { return ".webp" }

// Encoder registry
var encoderRegistry = map[string]ImageEncoder{
	"png":  PNGEncoder{},
	"webp": WebPEncoder{},
}

// GetEncoder returns encoder for given format
func GetEncoder(format string) (ImageEncoder, error) {
	format = strings.ToLower(format)
	if enc, ok := encoderRegistry[format]; ok {
		return enc, nil
	}
	return nil, fmt.Errorf("unsupported format: %s", format)
}

// toRGBA converts any image to RGBA
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba
}

// LoadedImage holds image data for processing
type LoadedImage struct {
	OrigName string      // Original filename for "original" format
	Img      *image.RGBA // Decoded RGBA (for conversion)
	RawData  []byte      // Original bytes (for "original" format)
	FileHash string
}

// ExtractImagesFromFile extracts images from a PDF
// For "original": saves native format with deduplication
// For "png"/"webp": decodes, converts, and encodes with concurrency
func ExtractImagesFromFile(filename string, imgDir string, format string) error {
	if err := os.Mkdir(imgDir, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	// Extract to temp directory
	tempDir, err := os.MkdirTemp("", "pdfimg")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := api.ExtractImagesFile(filename, tempDir, nil, nil); err != nil {
		return fmt.Errorf("extract images: %w", err)
	}

	// Load all images (single read per file)
	images, err := loadImages(tempDir)
	if err != nil {
		return err
	}

	if len(images) == 0 {
		return nil
	}

	// Deduplicate
	images = deduplicate(images)

	// Process based on format
	format = strings.ToLower(format)
	if format == "original" || format == "" {
		return saveOriginal(images, imgDir)
	}

	encoder, err := GetEncoder(format)
	if err != nil {
		return err
	}

	return saveConverted(images, imgDir, encoder)
}

// loadImages reads and decodes all image files
func loadImages(dir string) ([]LoadedImage, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var images []LoadedImage
	for _, f := range files {
		if !isImageFile(f.Name()) {
			continue
		}

		path := filepath.Join(dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name(), err)
		}

		// Decode and convert to RGBA
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			continue // Skip undecodable files
		}

		images = append(images, LoadedImage{
			OrigName: f.Name(),
			Img:      toRGBA(img),
			RawData:  data,
			FileHash: hashBytes(data),
		})
	}
	return images, nil
}

// deduplicate removes duplicate images by hash
func deduplicate(images []LoadedImage) []LoadedImage {
	seen := make(map[string]bool)
	var unique []LoadedImage

	for _, img := range images {
		if !seen[img.FileHash] {
			seen[img.FileHash] = true
			unique = append(unique, img)
		}
	}

	if len(unique) < len(images) {
		fmt.Printf("skipped %d duplicate(s)\n", len(images)-len(unique))
	}
	return unique
}

// saveOriginal copies raw files preserving original format
func saveOriginal(images []LoadedImage, imgDir string) error {
	for i, img := range images {
		ext := strings.ToLower(filepath.Ext(img.OrigName))
		if ext == "" {
			ext = ".png"
		}
		path := filepath.Join(imgDir, fmt.Sprintf("image_%04d%s", i+1, ext))
		if err := os.WriteFile(path, img.RawData, 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// saveConverted encodes images concurrently using all available CPUs
func saveConverted(images []LoadedImage, imgDir string, encoder ImageEncoder) error {
	numWorkers := runtime.NumCPU()

	type task struct {
		index int
		img   *image.RGBA
	}

	tasks := make(chan task, len(images))
	results := make(chan error, len(images))

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				results <- encodeImage(t.img, encoder, imgDir, t.index)
			}
		}()
	}

	// Dispatch tasks
	go func() {
		for i, img := range images {
			tasks <- task{index: i, img: img.Img}
		}
		close(tasks)
	}()

	// Wait and collect first error
	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	for err := range results {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// encodeImage encodes a single image to disk
func encodeImage(img *image.RGBA, encoder ImageEncoder, imgDir string, index int) error {
	buf := getBuffer()
	defer putBuffer(buf)

	if err := encoder.Encode(buf, img); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	outPath := filepath.Join(imgDir, fmt.Sprintf("image_%04d%s", index+1, encoder.Extension()))
	return os.WriteFile(outPath, buf.Bytes(), 0644)
}

// isImageFile checks if filename has image extension
func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" ||
		ext == ".gif" || ext == ".bmp" || ext == ".tiff" || ext == ".webp"
}

// hashBytes computes SHA-256 of data
func hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}
