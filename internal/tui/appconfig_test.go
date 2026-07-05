package tui

import (
	"os"
	"path/filepath"
	"testing"

	"pkm/internal/vault"
)

func TestFillConfigDefaultsBackfillsLegacyFile(t *testing.T) {
	// Simulates a config.yaml written before Keymap/Variables existed.
	cfg := AppConfig{Theme: "dracula", SidebarWidth: 20, RestoreSession: true}
	fillConfigDefaults(&cfg)

	if cfg.Keymap.Quit != "ctrl+q" || cfg.Keymap.Palette != "ctrl+@" {
		t.Fatalf("expected default keymap to be backfilled, got %+v", cfg.Keymap)
	}
	if cfg.Variables == nil {
		t.Fatal("expected Variables to be initialized to an empty map, got nil")
	}
	if cfg.Version != currentConfigVersion {
		t.Fatalf("expected version stamped to %d, got %d", currentConfigVersion, cfg.Version)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}

	cfg := defaultConfig()
	cfg.Theme = "gruvbox"
	cfg.Keymap.PanePicker = "ctrl+g"
	cfg.Variables["author"] = "Alex"

	if err := exportConfig(v, cfg, "myconfig.yaml"); err != nil {
		t.Fatalf("exportConfig: %v", err)
	}

	got, err := importConfig(v, "myconfig.yaml")
	if err != nil {
		t.Fatalf("importConfig: %v", err)
	}
	if got.Theme != "gruvbox" {
		t.Errorf("Theme = %q, want gruvbox", got.Theme)
	}
	if got.Keymap.PanePicker != "ctrl+g" {
		t.Errorf("Keymap.PanePicker = %q, want ctrl+g", got.Keymap.PanePicker)
	}
	if got.Variables["author"] != "Alex" {
		t.Errorf("Variables[author] = %q, want Alex", got.Variables["author"])
	}
}

func TestImportToleratesVersionMismatchAndUnknownFields(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}

	// A "future" export: higher version, an unknown field, and only a
	// partial set of known fields set. Must not crash and must fall back to
	// defaults for anything missing.
	future := "version: 99\ntheme: tokyonight\nsome_future_field:\n  nested: true\n"
	path := filepath.Join(v.Root, "future.yaml")
	if err := os.WriteFile(path, []byte(future), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := importConfig(v, "future.yaml")
	if err != nil {
		t.Fatalf("importConfig should not error on version mismatch, got: %v", err)
	}
	if got.Theme != "tokyonight" {
		t.Errorf("Theme = %q, want tokyonight", got.Theme)
	}
	if got.Keymap.Quit != "ctrl+q" {
		t.Errorf("expected default keymap for fields absent from the file, got %+v", got.Keymap)
	}
	if got.SidebarWidth != 25 {
		t.Errorf("SidebarWidth = %d, want default 25", got.SidebarWidth)
	}
}

func TestCmdInsertVarRequiresEditMode(t *testing.T) {
	m := setupTUI(t)
	m.cfg.Variables = map[string]string{"author": "Alex"}

	msg, _ := m.cmdInsertVar([]string{"author"})
	if msg == "" {
		t.Fatal("expected a usage/error message when not editing, got empty string")
	}
}

func TestCmdInsertVarUnknownName(t *testing.T) {
	m := setupTUI(t)
	m.cfg.Variables = map[string]string{}

	msg, _ := m.cmdInsertVar([]string{"missing"})
	if msg != `unknown variable: "missing"` {
		t.Fatalf("unexpected message: %q", msg)
	}
}
