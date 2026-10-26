// Package pdf extracts text and renders pages with Poppler.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Page contains text extracted from a PDF page.
type Page struct {
	Number int
	Text   string
}

// Image is a rendered PDF page.
type Image struct {
	Number int
	Mime   string
	Data   []byte
}

// Extractor reads text from a PDF.
type Extractor interface {
	Extract(ctx context.Context, path string) ([]Page, error)
}

// Renderer renders PDF pages as images.
type Renderer interface {
	Render(ctx context.Context, path string, dpi int) ([]Image, error)
}

// PdftotextExtractor uses pdftotext.
type PdftotextExtractor struct{}

func NewPdftotext() *PdftotextExtractor { return &PdftotextExtractor{} }

func (e *PdftotextExtractor) Extract(ctx context.Context, path string) ([]Page, error) {
	if err := validatePDF(path); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", path, "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, commandError("pdftotext", err, stderr.String())
	}

	pages := strings.Split(string(output), "\f")
	for len(pages) > 0 && strings.TrimSpace(pages[len(pages)-1]) == "" {
		pages = pages[:len(pages)-1]
	}
	extractedPages := make([]Page, 0, len(pages))
	for i, pageText := range pages {
		extractedPages = append(extractedPages, Page{Number: i + 1, Text: strings.TrimSpace(pageText)})
	}
	return extractedPages, nil
}

func validatePDF(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("PDF path must not be empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("open PDF %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("PDF path %q is not a regular file", path)
	}
	return nil
}

func commandError(name string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return fmt.Errorf("run %s: %w: %s", name, err, detail)
}
