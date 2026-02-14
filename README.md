# pixf

**PDF Image Extract** - A simple yet powerful PDF toolkit for unlocking PDFs and extracting images.

## Overview

pixf is a command-line tool built in Go that provides two primary functionalities:
- **Unlock PDFs**: Remove password protection from PDF files
- **Extract Images**: Extract all images from PDFs in your preferred format

## Features

- 🔓 **Unlock PDFs** - Remove password protection from encrypted PDF files
- 🖼️ **Extract Images** - Extract all images from PDF documents
- 📁 **Multiple Formats** - Extract as original format, PNG, or WebP
- 🚀 **Concurrent Processing** - Fast image extraction with parallel processing
- 🔄 **Deduplication** - Automatically removes duplicate images
- 🖥️ **Simple CLI** - Easy-to-use command-line interface

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/yourusername/pixf.git
cd pixf

# Build the project
go build -o pixf .

# Optional: Add to PATH
mv pixf /usr/local/bin/
```

### Binary Release

Download the latest binary from the [releases page](https://github.com/yourusername/pixf/releases).

## Usage

```bash
pixf [OPTIONS] <pdf-file> [format]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `pdf-file` | Path to the PDF file to process (required) |
| `format` | Image output format (optional, default: `original`) |

### Options

| Flag | Description |
|------|-------------|
| `-h, --help` | Show help message |
| `--unlock-only` | Only unlock the PDF, do not extract images |
| `--extract-only` | Only extract images, do not unlock the PDF first |

### Format Options

| Format | Description |
|--------|-------------|
| `original` | Extract images using PDF's native format (default) |
| `png` | Extract as PNG with transparency support |
| `webp` | Extract as WebP with transparency support |

## Examples

### Basic Usage

```bash
# Unlock PDF and extract images (original format)
pixf document.pdf

# Extract images as PNG
pixf document.pdf png

# Extract images as WebP
pixf document.pdf webp
```

### Unlock Only Mode

```bash
# Only unlock the PDF (no image extraction)
pixf --unlock-only document.pdf
```

### Extract Only Mode

```bash
# Only extract images without unlocking
pixf --extract-only document.pdf
```

### Show Help

```bash
# Show help message
pixf -h
pixf --help
```

## Output

- Unlocked PDFs are saved as `unlocked_<original-filename>`
- Extracted images are saved in `images_<pdf-name>/` directory
- Duplicate images are automatically detected and skipped

## Dependencies

- [pdfcpu](https://github.com/pdfcpu/pdfcpu) - PDF processing library
- [chai2010/webp](https://github.com/chai2010/webp) - WebP encoding support

## Roadmap

- [ ] PNG format with transparency (✅ implemented)
- [ ] Image upscale
- [ ] Image compression
- [ ] Image cropping
- [ ] Conversion to WebP (✅ implemented)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.


