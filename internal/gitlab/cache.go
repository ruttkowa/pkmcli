package gitlab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Cache struct {
	Version   int                `json:"version"`
	FetchedAt time.Time          `json:"fetched_at"`
	Projects  map[string][]Issue `json:"projects"`
}

func LoadCache(pkmDir string) Cache {
	data, err := os.ReadFile(filepath.Join(pkmDir, "issues.json"))
	if err != nil {
		return Cache{Version: 1, Projects: map[string][]Issue{}}
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return Cache{Version: 1, Projects: map[string][]Issue{}}
	}
	if cache.Projects == nil {
		cache.Projects = map[string][]Issue{}
	}
	cache.Version = 1
	return cache
}

func SaveCache(pkmDir string, cache Cache) error {
	cache.Version = 1
	if cache.Projects == nil {
		cache.Projects = map[string][]Issue{}
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pkmDir, "issues.json"), data, 0o644)
}
