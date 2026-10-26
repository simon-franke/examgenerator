package latex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilePDFIsolatedAndPreservesSource(t *testing.T) {
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = rm ]; then exit 0; fi
printf '%s\n' "$@" >> "$TEST_ARGS"
while [ "$#" -gt 0 ]; do
 if [ "$1" = --mount ]; then shift; dir=${1#type=bind,src=}; dir=${dir%,dst=/work}; fi
 shift
done
if [ "$TEST_FAIL" = yes ]; then exit 1; fi
printf '%%PDF-1.4\nfixture' > "$dir/exam.pdf"
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_ARGS", filepath.Join(bin, "args"))
	t.Setenv("TEST_FAIL", "")
	path := filepath.Join(t.TempDir(), "exam.tex")
	original := `\documentclass{article}\begin{document}test\end{document}`
	os.WriteFile(path, []byte(original), 0o600)
	out, err := CompilePDF(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := os.ReadFile(out)
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("missing PDF")
	}
	args, _ := os.ReadFile(filepath.Join(bin, "args"))
	for _, want := range []string{"--network=none", "--read-only", "--cap-drop=ALL", "--pull=never", "openin_any=p", "-no-shell-escape"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(args), filepath.Dir(path)) {
		t.Fatal("mounted project source directory")
	}
	t.Setenv("TEST_FAIL", "yes")
	if _, err := CompilePDF(context.Background(), path); err == nil {
		t.Fatal("expected compiler error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("source modified")
	}
	after, _ := os.ReadFile(out)
	if string(after) != string(pdf) {
		t.Fatal("failed compilation overwrote existing PDF")
	}
}
