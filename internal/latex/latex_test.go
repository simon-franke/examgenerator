package latex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanExtractsDocumentFromFenceAndChatter(t *testing.T) {
	raw := "Here it is:\n```latex\n\\documentclass{exam}\n\\begin{document}\nQ\n\\end{document}\n```\nDone"
	want := "\\documentclass{exam}\n\\begin{document}\nQ\n\\end{document}\n"
	if got := Clean(raw); got != want {
		t.Fatalf("Clean() = %q, want %q", got, want)
	}
}

func TestWriteCreatesParentAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "exam.tex")
	if err := Write(path, "document\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "document\n" {
		t.Fatalf("content = %q", data)
	}
}
