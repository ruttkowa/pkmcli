package installer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCopiesBinaryAndUpdatesPathIdempotently(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "pkm-download")
	if err := os.WriteFile(source, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".local", "bin", "pkm")
	opts := Options{
		Executable:  source,
		Destination: destination,
		Home:        home,
		Shell:       "/bin/zsh",
		In:          strings.NewReader("\n"),
		Out:         &bytes.Buffer{},
	}
	if err := Install(opts); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "binary" {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
	rc := filepath.Join(home, ".zshrc")
	first, _ := os.ReadFile(rc)
	opts.In = strings.NewReader("\n")
	if err := Install(opts); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(rc)
	if string(first) != string(second) {
		t.Fatalf("PATH line duplicated:\n%s", second)
	}
	if !strings.Contains(string(second), filepath.Join(home, ".local", "bin")) {
		t.Fatalf("rc missing install directory: %s", second)
	}
}

func TestInstallSkipsPromptWhenDestinationIsOnPath(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "pkm")
	os.WriteFile(source, []byte("binary"), 0o755)
	dir := filepath.Join(home, "bin")
	var out bytes.Buffer
	if err := Install(Options{
		Executable:  source,
		Destination: filepath.Join(dir, "pkm"),
		Home:        home,
		Shell:       "/bin/bash",
		Path:        dir,
		In:          strings.NewReader("n\n"),
		Out:         &out,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected rc update: %v", err)
	}
}
