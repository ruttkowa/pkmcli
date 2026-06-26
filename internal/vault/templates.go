package vault

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ApplyTemplate finds the first template in vault/templates/ and substitutes
// {{id}}, {{title}}, {{created}}, {{updated}}. Returns empty string if no
// template exists, in which case the caller should use bare frontmatter.
func (v *Vault) ApplyTemplate(id, title string, now time.Time) string {
	dir := filepath.Join(v.Root, "templates")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		body := string(data)
		body = strings.ReplaceAll(body, "{{id}}", id)
		body = strings.ReplaceAll(body, "{{title}}", title)
		body = strings.ReplaceAll(body, "{{created}}", now.Format("2006-01-02T15:04:05"))
		body = strings.ReplaceAll(body, "{{updated}}", now.Format("2006-01-02T15:04:05"))
		return body
	}
	return ""
}
