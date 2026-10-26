package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PdftoppmRenderer uses pdftoppm.
type PdftoppmRenderer struct{}

func NewPdftoppm() *PdftoppmRenderer { return &PdftoppmRenderer{} }

func (r *PdftoppmRenderer) Render(ctx context.Context, path string, dpi int) ([]Image, error) {
	if err := validatePDF(path); err != nil {
		return nil, err
	}
	if dpi <= 0 {
		return nil, fmt.Errorf("render DPI must be positive, got %d", dpi)
	}
	tempDir, err := os.MkdirTemp("", "examgen-render-")
	if err != nil {
		return nil, fmt.Errorf("create render directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	prefix := filepath.Join(tempDir, "page")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-png", "-r", strconv.Itoa(dpi), path, prefix)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, commandError("pdftoppm", err, stderr.String())
	}
	paths, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, fmt.Errorf("list rendered pages: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no pages for %q", path)
	}
	sort.Slice(paths, func(i, j int) bool { return renderedPageNumber(paths[i]) < renderedPageNumber(paths[j]) })

	images := make([]Image, 0, len(paths))
	for _, renderedPath := range paths {
		page := renderedPageNumber(renderedPath)
		if page <= 0 {
			return nil, fmt.Errorf("unexpected rendered page name %q", filepath.Base(renderedPath))
		}
		data, err := os.ReadFile(renderedPath)
		if err != nil {
			return nil, fmt.Errorf("read rendered page %d: %w", page, err)
		}
		images = append(images, Image{Number: page, Mime: "image/png", Data: data})
	}
	return images, nil
}

func renderedPageNumber(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dash := strings.LastIndexByte(name, '-')
	if dash < 0 {
		return 0
	}
	page, _ := strconv.Atoi(name[dash+1:])
	return page
}

var _ Renderer = (*PdftoppmRenderer)(nil)
