package gitlab

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cache := Cache{Version: 1, FetchedAt: time.Now().Truncate(time.Second), Projects: map[string][]Issue{"group/repo": {{IID: 3, Title: "three"}}}}
	if err := SaveCache(dir, cache); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(dir); !reflect.DeepEqual(got, cache) {
		t.Fatalf("LoadCache = %#v, want %#v", got, cache)
	}
}

func TestCorruptCacheReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "issues.json"), []byte("{"), 0o644)
	got := LoadCache(dir)
	if len(got.Projects) != 0 || !got.FetchedAt.IsZero() {
		t.Fatalf("cache = %#v", got)
	}
}
