package tui

import (
	"os"
	"path/filepath"

	"pkm/internal/vault"

	"gopkg.in/yaml.v3"
)

// AppConfig holds user preferences persisted to .pkm/config.yaml.
type AppConfig struct {
	Theme          string `yaml:"theme"`
	SidebarWidth   int    `yaml:"sidebar_width"`   // percent: 20, 25, 33
	RestoreSession bool   `yaml:"restore_session"` // restore last note on startup
	LineNumbers    bool   `yaml:"line_numbers"`    // show line numbers in editor
}

func defaultConfig() AppConfig {
	return AppConfig{
		Theme:          "nord",
		SidebarWidth:   25,
		RestoreSession: true,
		LineNumbers:    true,
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
	// Guard against zero/invalid values from a partial or hand-edited file.
	if cfg.SidebarWidth == 0 {
		cfg.SidebarWidth = 25
	}
	return cfg
}

func saveConfig(v *vault.Vault, cfg AppConfig) {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return
	}
	os.WriteFile(configFilePath(v), data, 0o644)
}
