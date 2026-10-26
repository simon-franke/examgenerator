// Package latex cleans and writes LaTeX source.
package latex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Clean extracts a LaTeX document from model output.
func Clean(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if fenceStart := strings.Index(cleaned, "```"); fenceStart >= 0 {
		afterFence := cleaned[fenceStart+3:]
		if newline := strings.IndexByte(afterFence, '\n'); newline >= 0 {
			language := strings.ToLower(strings.TrimSpace(afterFence[:newline]))
			if language == "" || language == "latex" || language == "tex" {
				afterFence = afterFence[newline+1:]
				if fenceEnd := strings.Index(afterFence, "```"); fenceEnd >= 0 {
					cleaned = afterFence[:fenceEnd]
				}
			}
		}
	}
	if documentStart := strings.Index(cleaned, `\documentclass`); documentStart >= 0 {
		if relativeEnd := strings.LastIndex(cleaned[documentStart:], `\end{document}`); relativeEnd >= 0 {
			documentEnd := documentStart + relativeEnd + len(`\end{document}`)
			cleaned = cleaned[documentStart:documentEnd]
		}
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	return cleaned + "\n"
}

// Write saves LaTeX source to path.
func Write(path, content string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path must not be empty")
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("LaTeX content must not be empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".examgen-*.tex")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set output permissions: %w", err)
	}
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace output %q: %w", path, err)
	}
	return nil
}
