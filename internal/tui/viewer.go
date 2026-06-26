package tui

import (
	"fmt"
	"strings"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type viewerModel struct {
	note        *vault.Note
	rendered    string // glamour-rendered output, cached in Update() via preRender()
	renderWidth int    // width used for the cached render
	scrollOff   int
	back        bool
}

func newViewer() viewerModel {
	return viewerModel{}
}

func (m viewerModel) withNote(n *vault.Note) viewerModel {
	m.note = n
	m.scrollOff = 0
	m.back = false
	m.rendered = ""
	m.renderWidth = 0
	return m
}

// preRender runs glamour and caches the result. Call this from Update() —
// NOT from render()/View() — so the cache persists across frames.
func (m viewerModel) preRender(width int) viewerModel {
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
	if err == nil {
		if out, err := r.Render(renderBody(m.note)); err == nil && out != "" {
			m.rendered = out
			m.renderWidth = width
			return m
		}
	}
	m.rendered = m.note.Body
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

	lines := strings.Split(rendered, "\n")
	start := m.scrollOff
	if start >= len(lines) {
		start = len(lines) - 1
	}
	if start < 0 {
		start = 0
	}
	// Reserve one row for the scroll indicator.
	contentRows := height - 1
	if contentRows < 1 {
		contentRows = 1
	}
	end := start + contentRows
	if end > len(lines) {
		end = len(lines)
	}
	visible := strings.Join(lines[start:end], "\n")

	indicator := "\n"
	if focused && len(lines) > 0 {
		pct := (start * 100) / len(lines)
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
		Render(visible + indicator)
}

// renderBody substitutes wikilinks for display and prepends a title header.
func renderBody(n *vault.Note) string {
	body := substituteLinks(n.Body)
	meta := fmt.Sprintf("**State:** %s", n.State)
	if n.Project != "" {
		meta += fmt.Sprintf("  •  **Project:** %s", n.Project)
	}
	if len(n.Tags) > 0 {
		meta += fmt.Sprintf("  •  **Tags:** #%s", strings.Join(n.Tags, " #"))
	}
	return fmt.Sprintf("# %s\n\n_%s_\n\n---\n\n%s", n.Title, meta, body)
}

// substituteLinks replaces [[Title|Alias]] → Alias and [[Title]] → Title.
func substituteLinks(body string) string {
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
		if pipe := strings.Index(inner, "|"); pipe != -1 {
			b.WriteString(inner[pipe+1:]) // alias
		} else {
			b.WriteString(inner) // title
		}
		body = rest[end+2:]
	}
	return b.String()
}
