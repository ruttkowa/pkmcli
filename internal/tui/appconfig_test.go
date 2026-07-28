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

// TestLoadConfigDefaultsShowNavFlagsForLegacyFile guards against issue #14's
// backward-compat trap: ShowTasksNav/ShowTemplatesNav are bools, so their Go
// zero value is false. A config.yaml written before these fields existed
// must NOT hide both nav rows on upgrade — loadConfig seeds defaultConfig()
// (both true) before unmarshalling, and yaml.Unmarshal only overwrites keys
// actually present in the file, so an absent key must leave the seeded true
// in place. This exercises the real loadConfig path end-to-end, not just
// fillConfigDefaults, since that's what production actually calls.
func TestLoadConfigDefaultsShowNavFlagsForLegacyFile(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}

	// A pre-#14 config file: no show_tasks_nav / show_templates_nav keys.
	legacy := "version: 1\ntheme: dracula\nsidebar_width: 20\nrestore_session: true\nline_numbers: true\n"
	if err := os.WriteFile(configFilePath(v), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := loadConfig(v)
	if !cfg.ShowTasksNav {
		t.Error("ShowTasksNav should default true for a config file predating the field")
	}
	if !cfg.ShowTemplatesNav {
		t.Error("ShowTemplatesNav should default true for a config file predating the field")
	}

	// An explicit false in the file must still be honored (bool false is a
	// legitimate saved preference, not just "field absent").
	explicit := legacy + "show_tasks_nav: false\nshow_templates_nav: false\n"
	if err := os.WriteFile(configFilePath(v), []byte(explicit), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg = loadConfig(v)
	if cfg.ShowTasksNav {
		t.Error("explicit show_tasks_nav: false in the file must be honored, not overridden back to true")
	}
	if cfg.ShowTemplatesNav {
		t.Error("explicit show_templates_nav: false in the file must be honored, not overridden back to true")
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

func TestGitLabConfigDefaultsRoundTripAndIssuesEditor(t *testing.T) {
	cfg := AppConfig{}
	fillConfigDefaults(&cfg)
	if cfg.GitLabURL != "https://gitlab.com" {
		t.Fatalf("default GitLab URL = %q", cfg.GitLabURL)
	}
	cfg.GitLabURL = "https://gitlab.example/"
	cfg.GitLabProjects = []string{"group/one"}
	fillConfigDefaults(&cfg)
	if cfg.GitLabURL != "https://gitlab.example" {
		t.Fatalf("trailing slash not stripped: %q", cfg.GitLabURL)
	}

	v, err := vault.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saveConfig(v, cfg)
	loaded := loadConfig(v)
	if loaded.GitLabURL != cfg.GitLabURL || len(loaded.GitLabProjects) != 1 ||
		loaded.GitLabProjects[0] != "group/one" {
		t.Fatalf("round trip = %#v", loaded)
	}

	pane := newConfigPane(loaded)
	pane.section = secIssues
	pane = pane.updateIssues(key("a"))
	for _, r := range "group/two" {
		pane = pane.updateIssues(key(string(r)))
	}
	pane = pane.updateIssues(key("enter"))
	if len(pane.gitlabProjects) != 2 {
		t.Fatalf("projects after add = %v", pane.gitlabProjects)
	}
	pane.issueCursor = 2
	pane = pane.updateIssues(key("d"))
	if len(pane.gitlabProjects) != 1 {
		t.Fatalf("projects after remove = %v", pane.gitlabProjects)
	}
}
