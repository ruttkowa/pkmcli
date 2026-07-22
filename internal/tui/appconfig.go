package tui

import (
	"os"
	"path/filepath"

	"pkm/internal/vault"

	"gopkg.in/yaml.v3"
)

// currentConfigVersion is bumped whenever AppConfig's shape changes in a way
// worth tracking. Import/export use it only informationally — loading never
// fails on a mismatch; unknown fields are ignored and missing ones default.
const currentConfigVersion = 2

// AppConfig holds user preferences persisted to .pkm/config.yaml.
type AppConfig struct {
	Version            int               `yaml:"version"`
	Theme              string            `yaml:"theme"`
	SidebarWidth       int               `yaml:"sidebar_width"`        // percent: 20, 25, 33
	RestoreSession     bool              `yaml:"restore_session"`      // restore last note on startup
	LineNumbers        bool              `yaml:"line_numbers"`         // show line numbers in editor
	ShowTasksNav       bool              `yaml:"show_tasks_nav"`       // show the sidebar's Tasks quick-link
	ShowTemplatesNav   bool              `yaml:"show_templates_nav"`   // show the sidebar's #templates section
	TrashRetentionDays int               `yaml:"trash_retention_days"` // #1: days a :delete'd note stays recoverable in :trash
	Keymap             Keymap            `yaml:"keymap"`
	Variables          map[string]string `yaml:"variables"` // user-defined {{name}} substitutions for :insert
}

// Keymap holds the remappable global keybindings. Values are bubbletea key
// strings (tea.KeyMsg.String(), e.g. "ctrl+p"). Only the global chords prone
// to terminal-multiplexer prefix collisions are remappable — mode-internal
// navigation (arrows, hjkl, etc.) stays fixed.
type Keymap struct {
	Palette    string `yaml:"palette"` // open command palette (":" always works too)
	PanePicker string `yaml:"pane_picker"`
	NextPane   string `yaml:"next_pane"`
	Quit       string `yaml:"quit"`
	Undo       string `yaml:"undo"`
	Redo       string `yaml:"redo"`
	Save       string `yaml:"save"` // save note while editing
}

func defaultKeymap() Keymap {
	return Keymap{
		Palette:    "ctrl+@", // terminals report Ctrl+Space as ctrl+@
		PanePicker: "ctrl+p",
		NextPane:   "ctrl+w",
		Quit:       "ctrl+q",
		Undo:       "ctrl+z",
		Redo:       "ctrl+y",
		Save:       "ctrl+s",
	}
}

// keymapLabels and the slice/struct converters below let the config UI treat
// the keymap as a flat, indexable list without reflection.
var keymapLabels = []string{
	"Command palette",
	"Pane picker",
	"Next pane",
	"Quit",
	"Undo",
	"Redo",
	"Save (editor)",
}

func keymapToSlice(k Keymap) []string {
	return []string{k.Palette, k.PanePicker, k.NextPane, k.Quit, k.Undo, k.Redo, k.Save}
}

func sliceToKeymap(s []string) Keymap {
	get := func(i int) string {
		if i < len(s) {
			return s[i]
		}
		return ""
	}
	return Keymap{
		Palette:    get(0),
		PanePicker: get(1),
		NextPane:   get(2),
		Quit:       get(3),
		Undo:       get(4),
		Redo:       get(5),
		Save:       get(6),
	}
}

func defaultConfig() AppConfig {
	return AppConfig{
		Version:            currentConfigVersion,
		Theme:              "nord",
		SidebarWidth:       25,
		RestoreSession:     true,
		LineNumbers:        true,
		ShowTasksNav:       true,
		ShowTemplatesNav:   true,
		TrashRetentionDays: vault.DefaultRetention,
		Keymap:             defaultKeymap(),
		Variables:          map[string]string{},
	}
}

func configFilePath(v *vault.Vault) string {
	return filepath.Join(v.Root, ".pkm", "config.yaml")
}

func loadConfig(v *vault.Vault) AppConfig {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFilePath(v))
	if err != nil {
		return cfg
	}
	yaml.Unmarshal(data, &cfg)
	fillConfigDefaults(&cfg)
	return cfg
}

// fillConfigDefaults guards against zero/invalid values from a partial or
// hand-edited config file — including one written by an older version that
// predates a field, or a mismatched version imported from elsewhere.
func fillConfigDefaults(cfg *AppConfig) {
	if cfg.SidebarWidth == 0 {
		cfg.SidebarWidth = 25
	}
	if cfg.TrashRetentionDays <= 0 {
		cfg.TrashRetentionDays = vault.DefaultRetention
	}
	d := defaultKeymap()
	if cfg.Keymap.Palette == "" {
		cfg.Keymap.Palette = d.Palette
	}
	if cfg.Keymap.PanePicker == "" {
		cfg.Keymap.PanePicker = d.PanePicker
	}
	if cfg.Keymap.NextPane == "" {
		cfg.Keymap.NextPane = d.NextPane
	}
	if cfg.Keymap.Quit == "" {
		cfg.Keymap.Quit = d.Quit
	}
	if cfg.Keymap.Undo == "" {
		cfg.Keymap.Undo = d.Undo
	}
	if cfg.Keymap.Redo == "" {
		cfg.Keymap.Redo = d.Redo
	}
	if cfg.Keymap.Save == "" {
		cfg.Keymap.Save = d.Save
	}
	if cfg.Variables == nil {
		cfg.Variables = map[string]string{}
	}
	cfg.Version = currentConfigVersion
}

func saveConfig(v *vault.Vault, cfg AppConfig) {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return
	}
	os.WriteFile(configFilePath(v), data, 0o644)
}

const defaultConfigExportName = "config-export.yaml"

// resolveConfigPath resolves a user-supplied path against the vault root,
// falling back to a default filename when path is empty.
func resolveConfigPath(v *vault.Vault, path string) string {
	if path == "" {
		path = defaultConfigExportName
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(v.Root, path)
}

// exportConfig writes the current config to path (relative paths resolve
// against the vault root).
func exportConfig(v *vault.Vault, cfg AppConfig, path string) error {
	cfg.Version = currentConfigVersion
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(resolveConfigPath(v, path), data, 0o644)
}

// importConfig reads a config file from path and merges it onto the current
// defaults: unknown fields in the file are ignored, missing fields keep their
// default value, and a version mismatch never causes a crash — it's just
// best-effort field-by-field application.
func importConfig(v *vault.Vault, path string) (AppConfig, error) {
	data, err := os.ReadFile(resolveConfigPath(v, path))
	if err != nil {
		return AppConfig{}, err
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, err
	}
	fillConfigDefaults(&cfg)
	return cfg, nil
}
