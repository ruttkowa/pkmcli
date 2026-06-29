package tui

import (
	"fmt"
	"strings"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// linkRef holds the navigation target for one wikilink in parse order.
type linkRef struct {
	target string // note title (may not exist yet)
}

type viewerModel struct {
	note            *vault.Note
	rendered        string // glamour-rendered body (below header), cached
	renderedHeader  string // glamour-rendered sticky header, cached
	headerLineCount int    // number of lines in renderedHeader
	renderWidth     int    // width used for the cached render
	scrollOff       int
	back            bool
	linkLines       map[int]string // body-relative line index → note title to navigate to
}

func newViewer() viewerModel {
	return viewerModel{}
}

func (m viewerModel) withNote(n *vault.Note) viewerModel {
	m.note = n
	m.scrollOff = 0
	m.back = false
	m.rendered = ""
	m.renderedHeader = ""
	m.headerLineCount = 0
	m.renderWidth = 0
	m.linkLines = nil
	return m
}

// preRender runs glamour and caches the result. Call this from Update() —
// NOT from render()/View() — so the cache persists across frames.
func (m viewerModel) preRender(width int, titles map[string]bool) viewerModel {
	if m.note == nil {
		return m
	}
	if m.rendered != "" && m.renderWidth == width {
		return m // already cached for this width
	}
	glamourStyle := "dark"
	if activeTheme.Name == "light" {
		glamourStyle = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		m.rendered = m.note.Body
		m.renderWidth = width
		return m
	}

	// Render sticky header (title + meta + separator) separately.
	if out, herr := r.Render(buildHeaderMd(m.note)); herr == nil {
		m.renderedHeader = strings.TrimRight(out, "\n")
		m.headerLineCount = strings.Count(m.renderedHeader, "\n") + 1
	}

	// Render scrollable body.
	body, refs := buildBodyMd(m.note, titles)
	if out, berr := r.Render(body); berr == nil && out != "" {
		m.rendered, m.linkLines = processRenderedLinks(out, refs)
	} else {
		m.rendered = m.note.Body
	}
	m.renderWidth = width
	return m
}

func (m viewerModel) update(msg tea.KeyMsg) (viewerModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.scrollOff++
	case "k", "up":
		if m.scrollOff > 0 {
			m.scrollOff--
		}
	case "esc", "backspace":
		m.back = true
	}
	return m, nil
}

func (m viewerModel) render(width, height int, focused bool) string {
	if m.note == nil {
		return lipgloss.NewStyle().Width(width).Height(height).Background(activeTheme.Bg).Render("")
	}

	// Use the cache populated by preRender() in Update(). This function must
	// NOT call glamour — it runs on every event including mouse motion.
	rendered := m.rendered
	if rendered == "" {
		rendered = m.note.Body // cold cache: show raw until Update() warms it
	}

	bodyLines := strings.Split(rendered, "\n")

	// Reserve one row for the scroll indicator.
	contentRows := height - 1
	if contentRows < 1 {
		contentRows = 1
	}

	var sb strings.Builder

	// Sticky header: always rendered at the top.
	if m.renderedHeader != "" {
		headerLines := strings.Split(m.renderedHeader, "\n")
		headerRows := m.headerLineCount
		if headerRows > contentRows {
			headerRows = contentRows
		}
		sb.WriteString(strings.Join(headerLines[:headerRows], "\n"))
		contentRows -= headerRows
		// Fold separator line.
		if contentRows > 0 {
			sep := lipgloss.NewStyle().
				Foreground(activeTheme.BorderNormal).
				Render(strings.Repeat("─", width-2))
			sb.WriteByte('\n')
			sb.WriteString(sep)
			contentRows--
		}
		if contentRows > 0 {
			sb.WriteByte('\n')
		}
	}

	// Scrollable body below the header.
	start := m.scrollOff
	if start >= len(bodyLines) {
		start = len(bodyLines) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + contentRows
	if end > len(bodyLines) {
		end = len(bodyLines)
	}
	if contentRows > 0 {
		sb.WriteString(strings.Join(bodyLines[start:end], "\n"))
	}

	indicator := "\n"
	if focused && len(bodyLines) > 0 {
		pct := (start * 100) / len(bodyLines)
		indicator = "\n" + lipgloss.NewStyle().
			Width(width - 2).
			Foreground(activeTheme.TextDim).
			Align(lipgloss.Right).
			Render(fmt.Sprintf("%d%%", pct))
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(activeTheme.Bg).
		Padding(0, 1).
		Render(sb.String() + indicator)
}

// linkAtLine returns the note title linked on a given rendered line index, or "".
func (m viewerModel) linkAtLine(renderedLine int) string {
	if m.linkLines == nil {
		return ""
	}
	return m.linkLines[renderedLine]
}

// buildHeaderMd returns the markdown for the sticky header (title + meta).
func buildHeaderMd(n *vault.Note) string {
	meta := fmt.Sprintf("**State:** %s", n.State)
	if n.Project != "" {
		meta += fmt.Sprintf("  •  **Project:** %s", n.Project)
	}
	if len(n.Tags) > 0 {
		meta += fmt.Sprintf("  •  **Tags:** #%s", strings.Join(n.Tags, " #"))
	}
	return fmt.Sprintf("# %s\n\n_%s_", n.Title, meta)
}

// buildBodyMd substitutes wikilinks and returns the body-only markdown for glamour.
func buildBodyMd(n *vault.Note, titles map[string]bool) (string, []linkRef) {
	return substituteLinks(n.Body, titles)
}

// substituteLinks replaces [[Title|Alias]] / [[Title]] with glamour-ready markdown.
// Working links become [·Alias·]() (glamour renders as colored link).
// Broken links become ~~Alias~~ (glamour renders as strikethrough).
// Both are wrapped in PUA sentinels for click-position tracking.
// Spaces in aliases are replaced with nbsp to prevent glamour mid-token wrapping.
func substituteLinks(body string, titles map[string]bool) (string, []linkRef) {
	const (
		wOpen  = "" // working link open sentinel
		wClose = "" // working link close sentinel
		bOpen  = "" // broken link open sentinel
		bClose = "" // broken link close sentinel
	)

	var refs []linkRef
	var b strings.Builder

	for {
		start := strings.Index(body, "[[")
		if start == -1 {
			b.WriteString(body)
			break
		}
		b.WriteString(body[:start])
		rest := body[start+2:]
		end := strings.Index(rest, "]]")
		if end == -1 {
			b.WriteString("[[")
			body = rest
			continue
		}
		inner := rest[:end]
		var target, alias string
		if pipe := strings.Index(inner, "|"); pipe != -1 {
			target = strings.TrimSpace(inner[:pipe])
			alias = strings.TrimSpace(inner[pipe+1:])
		} else {
			target = strings.TrimSpace(inner)
			alias = target
		}

		exists := len(titles) > 0 && titles[strings.ToLower(target)]
		refs = append(refs, linkRef{target: target})

		// Replace spaces with nbsp so glamour won't wrap mid-alias.
		display := strings.ReplaceAll(alias, " ", " ")

		if exists {
			b.WriteString(wOpen + "[·" + display + "·]()" + wClose)
		} else {
			b.WriteString(bOpen + "~~" + display + "~~" + bClose)
		}

		body = rest[end+2:]
	}
	return b.String(), refs
}

// processRenderedLinks walks the glamour output, strips PUA sentinels, and
// builds a map of rendered-line-index → note target for click detection.
// Sentinel pairs are matched to refs in left-to-right parse order.
func processRenderedLinks(rendered string, refs []linkRef) (string, map[int]string) {
	const (
		wOpen  = ""
		wClose = ""
		bOpen  = ""
		bClose = ""
	)

	lines := strings.Split(rendered, "\n")
	linkMap := make(map[int]string)
	refIdx := 0

	for lineIdx, line := range lines {
		for {
			// Find whichever sentinel pair opens first on this line.
			wIdx := strings.Index(line, wOpen)
			bIdx := strings.Index(line, bOpen)

			open, openS, closeS := -1, "", ""
			switch {
			case wIdx != -1 && (bIdx == -1 || wIdx < bIdx):
				open, openS, closeS = wIdx, wOpen, wClose
			case bIdx != -1:
				open, openS, closeS = bIdx, bOpen, bClose
			}
			if open == -1 {
				break
			}

			rest := line[open+len(openS):]
			closeIdx := strings.Index(rest, closeS)
			if closeIdx == -1 {
				break // sentinel split across line — skip (rare with nbsp)
			}

			// Record first link target found on this line.
			if _, seen := linkMap[lineIdx]; !seen && refIdx < len(refs) {
				linkMap[lineIdx] = refs[refIdx].target
			}
			if refIdx < len(refs) {
				refIdx++
			}

			// Strip the sentinel chars; keep the glamour-styled content between.
			content := rest[:closeIdx]
			line = line[:open] + content + rest[closeIdx+len(closeS):]
		}
		lines[lineIdx] = line
	}

	return strings.Join(lines, "\n"), linkMap
}
