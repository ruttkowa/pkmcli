package vault

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const fmDelimiter = "---"

// parseNote splits a raw .md file into frontmatter + body and populates n.
func parseNote(raw []byte, n *Note) error {
	content := strings.TrimPrefix(string(raw), "\xef\xbb\xbf") // strip BOM

	if !strings.HasPrefix(content, fmDelimiter) {
		n.Body = content
		return nil
	}

	// Find closing delimiter
	rest := content[len(fmDelimiter):]
	end := strings.Index(rest, "\n"+fmDelimiter)
	if end == -1 {
		return fmt.Errorf("unclosed frontmatter delimiter")
	}

	fmRaw := rest[:end]
	n.Body = strings.TrimPrefix(rest[end+len("\n"+fmDelimiter):], "\n")

	return yaml.Unmarshal([]byte(fmRaw), n)
}

// marshalNote serializes a note back to raw .md bytes.
func marshalNote(n *Note) ([]byte, error) {
	fm, err := yaml.Marshal(n)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(fmDelimiter + "\n")
	buf.Write(fm)
	buf.WriteString(fmDelimiter + "\n")
	if n.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(n.Body)
	}
	return buf.Bytes(), nil
}
