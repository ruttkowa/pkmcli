package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPathBackupIsPortableAndUpdatesAtomically(t *testing.T) {
	root := t.TempDir()
	destination := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Mode: "path", Destination: destination, Timeout: 10 * time.Second}
	if err := Run(context.Background(), root, cfg); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(destination, filepath.Base(root)+".bundle")
	cmd := exec.Command("git", "bundle", "verify", bundlePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify bundle: %v\n%s", err, output)
	}
	if _, err := os.Stat(bundlePath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary bundle remains: %v", err)
	}
}

func TestBackupPathCannotBeInsideVault(t *testing.T) {
	root := t.TempDir()
	err := Run(context.Background(), root, Config{
		Mode:        "path",
		Destination: filepath.Join(root, "backup"),
		Timeout:     10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected unsafe nested destination to fail")
	}
}

func TestUnavailablePathIsNotCreated(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(t.TempDir(), "missing-mount")
	err := Run(context.Background(), root, Config{
		Mode: "path", Destination: destination, Timeout: 10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected unavailable destination to fail")
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("unavailable destination was created: %v", statErr)
	}
}

func TestRemoteBackupPushesWithoutPrompt(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	cmd := exec.Command("git", "init", "--bare", remote)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), root, Config{
		Mode: "remote", Destination: remote, Timeout: 10 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "--git-dir", remote, "rev-parse", "--verify", "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote HEAD: %v\n%s", err, output)
	}
}
