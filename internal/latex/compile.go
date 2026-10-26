package latex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CompilerImage is deliberately not pulled automatically. Setup is an explicit user action.
const CompilerImage = "ghcr.io/xu-cheng/texlive-full:latest"

// CheckCompiler fails before generation if Docker or the prepared image is unavailable.
func CheckCompiler(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", CompilerImage).Run(); err != nil {
		return fmt.Errorf("PDF compiler unavailable: start Docker and run docker pull --platform linux/amd64 %s: %w", CompilerImage, err)
	}
	return nil
}

// CompilePDF compiles in a disposable container with no network or host credentials.
// Only a fresh directory containing the source is mounted, never the project directory.
func CompilePDF(ctx context.Context, sourcePath string) (string, error) {
	if !strings.EqualFold(filepath.Ext(sourcePath), ".tex") {
		return "", fmt.Errorf("PDF compilation requires a .tex output path")
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "examgen-compile-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err = os.WriteFile(filepath.Join(dir, "exam.tex"), source, 0o600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	name := filepath.Base(dir)
	// Explicit cleanup also handles interruption of the Docker client before --rm runs.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
	}()
	args := []string{"run", "--platform=linux/amd64", "--rm", "--pull=never", "--name", name, "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=512m", "--cpus=1", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--tmpfs", "/tmp:rw,nosuid,size=128m", "--mount", "type=bind,src=" + dir + ",dst=/work", "--workdir", "/work", "--env", "openin_any=p", "--env", "openout_any=p", "--entrypoint", "pdflatex", CompilerImage, "-no-shell-escape", "-interaction=nonstopmode", "-halt-on-error", "exam.tex"}
	for pass := 0; pass < 2; pass++ {
		cmd := exec.CommandContext(ctx, "docker", args...)
		var diagnostics compilerOutput
		cmd.Stdout, cmd.Stderr = &diagnostics, &diagnostics
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("PDF compilation failed: %w; LaTeX remains at %s\n%s", err, sourcePath, string(diagnostics))
		}
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "exam.pdf"))
	if err != nil {
		return "", fmt.Errorf("read compiled PDF: %w", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		return "", fmt.Errorf("compiler did not produce a PDF")
	}
	out := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".pdf"
	temp, err := os.CreateTemp(filepath.Dir(out), ".examgen-*.pdf")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(pdf); err != nil {
		temp.Close()
		return "", err
	}
	if err = temp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(temp.Name(), out); err != nil {
		return "", err
	}
	return out, nil
}

// Retain bounded diagnostics even if malformed TeX emits excessive output.
type compilerOutput []byte

func (b *compilerOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 8192
	if len(p) > limit {
		p = p[len(p)-limit:]
	}
	*b = append(*b, p...)
	if len(*b) > limit {
		*b = (*b)[len(*b)-limit:]
	}
	return n, nil
}
