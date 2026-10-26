package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenvDoesNotOverrideEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXAMGEN_TEST_EXISTING=file\nEXAMGEN_TEST_NEW=\"hello world\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXAMGEN_TEST_EXISTING", "process")
	t.Setenv("EXAMGEN_TEST_NEW", "")
	if err := os.Unsetenv("EXAMGEN_TEST_NEW"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("EXAMGEN_TEST_NEW") })
	if err := loadDotenv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("EXAMGEN_TEST_EXISTING"); got != "process" {
		t.Fatalf("existing = %q", got)
	}
	if got := os.Getenv("EXAMGEN_TEST_NEW"); got != "hello world" {
		t.Fatalf("new = %q", got)
	}
}

func TestSlidePathsReadsAndSortsPDFs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.PDF", "a.pdf", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := slidePaths("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || filepath.Base(paths[0]) != "a.pdf" || filepath.Base(paths[1]) != "b.PDF" {
		t.Fatalf("paths = %#v", paths)
	}
}
