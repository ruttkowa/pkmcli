package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"pkm/internal/vault"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// linkRef holds the navigation target for one wikilink in parse order.
type linkRef struct {
	target string // note title (may not exist yet)
}

// checkboxLineRe matches a Markdown task-list item: "- [ ] text" / "- [x] text".
var checkboxLineRe = regexp.MustCompile(`^(\s*[-*]\s+)\[([ xX])\](.*)$`)

// taskDateRe matches the Obsidian-Tasks-style completion-date stamp
// (e.g. "✅ 2026-07-10") that toggleCheckboxLine adds to a task line when
// it's marked done, wherever it appears in the line.
var taskDateRe = regexp.MustCompile(`\s*✅\s*(\d{4}-\d{2}-\d{2})`)

// taskResultSep separates task text from its result: "- [x] Task --> result".
const taskResultSep = " --> "

// codeSpan is a fenced code block located in the rendered body.
type codeSpan struct {
	startLine int // rendered line index (body-relative) of the first content line
	endLine   int // rendered line index of the last content line
	content   string
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
	checkboxLines   map[int]int    // body-relative rendered line index → raw body line index
	codeSpans       []codeSpan     // fenced code blocks, in rendered-line order
	headingLines    map[int]int    // body-relative rendered line index → raw heading line index (#20)

	// rawLines is the general rendered→raw line map (#22), used to carry the
	// cursor position across the View/Edit boundary. Best-effort and
	// line-level only (see rawLineAt/renderedLineForRaw) — glamour's
	// word-wrap means most rendered lines have no exact raw counterpart, so
	// this only anchors the *start* of each heading/checkbox/plain-paragraph
	// block and forward-fills the rest, same technique as headingLines.
	rawLines map[int]int

	// folded holds this note's collapsed headings (#20), keyed by raw body
	// line index — view-only, never persisted or written to the note. Reset
	// on withNote (switching notes), but explicitly preserved by the two
	// same-note reload paths that already save/restore cursor/scroll
	// (vaultChangedMsg's checkbox-toggle reload, commitEditorDraft's
	// post-save reload) — otherwise every checkbox toggle would silently
	// un-fold the note.
	folded map[int]bool

	// Character-level block cursor (arrow keys), body-relative like scrollOff.
	cursorRow int
	cursorCol int

	// Text selection (#2), view-mode only. selAnchorRow/Col is where the
	// selection started; the live end is always cursorRow/cursorCol. Set by
	// Shift+Arrow (keyboard) or a mouse press over plain body text followed
	// by drag (model.go's handleMouseDrag/handleMouseRelease).
	selAnchorRow int
	selAnchorCol int
	selActive    bool
	dragging     bool // true while a mouse button is held after a body-text press

	// Set by update() when Enter activates whatever's under the cursor;
	// model.go's dispatcher consumes and clears these on the next frame.
	pendingLinkOpen    string // note title to open, or ""
	pendingCheckboxRaw int    // raw body line to toggle, or -1
	pendingCodeCopy    string // code block content to copy, or ""
	pendingCopyText    string // selected raw text to copy (#2, Ctrl+C), or ""

	// Set by update() when Left/Right collapses/expands a heading under the
	// cursor (#20); model.go's dispatcher consumes and clears this, since
	// applying it requires re-rendering (preRender, titles) that this
	// package-internal update() doesn't have access to.
	pendingFoldRaw      int  // raw heading line to fold/unfold, or -1
	pendingFoldCollapse bool // desired state when pendingFoldRaw >= 0
}

func newViewer() viewerModel {
	return viewerModel{pendingCheckboxRaw: -1, pendingFoldRaw: -1}
}

// renderMarkdownDocument is the shared Glamour setup for note bodies and
// virtual read-only documents such as GitLab issue details.
func renderMarkdownDocument(markdown string, width int) string {
	base := glamourstyles.DarkStyleConfig
	if activeTheme.Name == "solarized-light" {
		base = glamourstyles.LightStyleConfig
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(headingStyleConfig(base)),
		glamour.WithWordWrap(max(1, width-4)),
	)
	if err != nil {
		return markdown
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return rendered
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
	m.checkboxLines = nil
	m.codeSpans = nil
	m.headingLines = nil
	m.rawLines = nil
	m.folded = nil
	m.cursorRow = 0
	m.cursorCol = 0
	m.selAnchorRow = 0
	m.selAnchorCol = 0
	m.selActive = false
	m.dragging = false
	m.pendingLinkOpen = ""
	m.pendingCheckboxRaw = -1
	m.pendingCodeCopy = ""
	m.pendingCopyText = ""
	m.pendingFoldRaw = -1
	m.pendingFoldCollapse = false
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
	base := glamourstyles.DarkStyleConfig
	if activeTheme.Name == "solarized-light" {
		base = glamourstyles.LightStyleConfig
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(headingStyleConfig(base)),
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
	body, linkRefs, checkboxRefs, codeRefs, headingRefs, lineRefs := buildBodyMd(m.note, titles, m.folded)
	if out, berr := r.Render(body); berr == nil && out != "" {
		out, m.linkLines = processRenderedLinks(out, linkRefs)
		m.rendered, m.checkboxLines, m.codeSpans, m.headingLines, m.rawLines = processCheckboxesAndCode(out, checkboxRefs, codeRefs, headingRefs, lineRefs)
	} else {
		m.rendered = m.note.Body
	}
	m.renderWidth = width

	// Clamp the cursor into the freshly rendered content — it may point past
	// the end after an edit shortened the note, or after opening a new note.
	lines := strings.Split(m.rendered, "\n")
	if m.cursorRow >= len(lines) {
		m.cursorRow = max(0, len(lines)-1)
	}
	if m.cursorRow >= 0 && m.cursorRow < len(lines) {
		m.cursorCol = clampCol(m.cursorCol, lines[m.cursorRow])
	}
	return m
}

// visibleBodyRows returns how many body rows are visible at the given pane
// height, mirroring render()'s own accounting for the sticky header, fold
// separator, and scroll indicator — kept in sync so cursor auto-scroll
// matches what's actually drawn.
func (m viewerModel) visibleBodyRows(height int) int {
	contentRows := height - 1
	if contentRows < 1 {
		contentRows = 1
	}
	if m.renderedHeader != "" {
		headerRows := m.headerLineCount
		if headerRows > contentRows {
			headerRows = contentRows
		}
		contentRows -= headerRows
		if contentRows > 0 {
			contentRows--
		}
	}
	if contentRows < 1 {
		contentRows = 1
	}
	return contentRows
}

// followCursor adjusts scrollOff so cursorRow stays within the visible window.
func (m viewerModel) followCursor(height int) viewerModel {
	rows := m.visibleBodyRows(height)
	if m.cursorRow < m.scrollOff {
		m.scrollOff = m.cursorRow
	} else if m.cursorRow >= m.scrollOff+rows {
		m.scrollOff = m.cursorRow - rows + 1
	}
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
	return m
}

// clampScroll bounds scrollOff to the current rendered content — needed
// after a fold collapse (#20) shortens the content out from under a
// scroll position that was valid a moment ago, to avoid an out-of-range
// slice when render() draws the body starting at scrollOff.
func (m viewerModel) clampScroll() viewerModel {
	lines := strings.Split(m.rendered, "\n")
	maxScroll := max(0, len(lines)-1)
	if m.scrollOff > maxScroll {
		m.scrollOff = maxScroll
	}
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
	return m
}

// rawLineAt returns the raw body-line index the given rendered body line
// most likely corresponds to (#22, best-effort, line-level — see rawLines'
// doc comment). Falls back to the nearest preceding mapped rendered line,
// then to 0 if nothing is mapped yet (e.g. cursor sits before the first
// anchor).
func (m viewerModel) rawLineAt(renderedLine int) int {
	for i := renderedLine; i >= 0; i-- {
		if raw, ok := m.rawLines[i]; ok {
			return raw
		}
	}
	return 0
}

// renderedLineForRaw inverts rawLines: the first rendered line whose raw
// counterpart is rawLine, or — if that raw line was never its own anchor
// (a wrapped/merged continuation line within a block) — the rendered start
// of the nearest preceding raw line that was (#22's explicit fallback).
// rawLines is monotonically non-decreasing in rendered-line order (raw
// lines are scanned top-to-bottom), so a single forward pass suffices.
func (m viewerModel) renderedLineForRaw(rawLine int) int {
	lines := strings.Split(m.rendered, "\n")
	best := 0
	for i := range lines {
		raw, ok := m.rawLines[i]
		if !ok {
			continue
		}
		if raw == rawLine {
			return i
		}
		if raw > rawLine {
			break
		}
		best = i
	}
	return best
}

// clampCol bounds col to a line's display width (0..width, where width itself
// is a valid "past the last character" cursor position).
func clampCol(col int, line string) int {
	w := xansi.StringWidth(line)
	if col > w {
		return w
	}
	if col < 0 {
		return 0
	}
	return col
}

// activateCursor sets the pending* field matching whatever's under the
// cursor, for model.go's dispatcher to act on. No-op if the cursor sits over
// plain text.
func (m viewerModel) activateCursor() viewerModel {
	if target := m.linkAtLine(m.cursorRow); target != "" {
		m.pendingLinkOpen = target
		return m
	}
	if rawLine, ok := m.checkboxRawLineAt(m.cursorRow); ok {
		m.pendingCheckboxRaw = rawLine
		return m
	}
	if cs, ok := m.codeSpanAt(m.cursorRow); ok {
		m.pendingCodeCopy = cs.content
		return m
	}
	return m
}

// update handles a key while the note viewer has focus. height is the pane's
// content height, needed to keep cursor auto-scroll (followCursor) in sync
// with what render() will actually draw.
func (m viewerModel) update(msg tea.KeyMsg, height int) (viewerModel, tea.Cmd) {
	lines := strings.Split(m.rendered, "\n")

	switch msg.String() {
	case "j":
		m.scrollOff++
	case "k":
		if m.scrollOff > 0 {
			m.scrollOff--
		}
	case "shift+up", "shift+down", "shift+left", "shift+right":
		if !m.selActive {
			m.selAnchorRow, m.selAnchorCol = m.cursorRow, m.cursorCol
			m.selActive = true
		}
		m = m.moveCursorChar(strings.TrimPrefix(msg.String(), "shift+"), lines)
		m = m.followCursor(height)
	case "ctrl+a":
		if len(lines) > 0 {
			m.selAnchorRow, m.selAnchorCol = 0, 0
			m.cursorRow = len(lines) - 1
			m.cursorCol = xansi.StringWidth(lines[m.cursorRow])
			m.selActive = true
			m = m.followCursor(height)
		}
	case "ctrl+c":
		if m.selActive {
			m.pendingCopyText = m.selectedRawText()
		}
	case "down":
		m.selActive = false
		if m.cursorRow < len(lines)-1 {
			m.cursorRow++
			m.cursorCol = clampCol(m.cursorCol, lines[m.cursorRow])
		}
		m = m.followCursor(height)
	case "up":
		m.selActive = false
		if m.cursorRow > 0 {
			m.cursorRow--
			m.cursorCol = clampCol(m.cursorCol, lines[m.cursorRow])
		}
		m = m.followCursor(height)
	case "left":
		m.selActive = false
		// #20: on a heading line, Left collapses it instead of moving the
		// cursor — horizontal movement on a heading line is otherwise the
		// least valuable key in the app. Actually applying the fold needs a
		// re-render (preRender, titles) this package-internal update() has
		// no access to, so it's deferred via pendingFoldRaw to model.go's
		// dispatcher, same pattern as pendingLinkOpen/pendingCheckboxRaw.
		if rawLine, ok := m.headingRawLineAt(m.cursorRow); ok {
			m.pendingFoldRaw = rawLine
			m.pendingFoldCollapse = true
			break
		}
		if m.cursorCol > 0 {
			m.cursorCol--
		} else if m.cursorRow > 0 {
			m.cursorRow--
			m.cursorCol = xansi.StringWidth(lines[m.cursorRow])
		}
		m = m.followCursor(height)
	case "right":
		m.selActive = false
		if rawLine, ok := m.headingRawLineAt(m.cursorRow); ok {
			m.pendingFoldRaw = rawLine
			m.pendingFoldCollapse = false
			break
		}
		curWidth := 0
		if m.cursorRow >= 0 && m.cursorRow < len(lines) {
			curWidth = xansi.StringWidth(lines[m.cursorRow])
		}
		if m.cursorCol < curWidth {
			m.cursorCol++
		} else if m.cursorRow < len(lines)-1 {
			m.cursorRow++
			m.cursorCol = 0
		}
		m = m.followCursor(height)
	case "enter":
		m = m.activateCursor()
	case " ":
		if rawLine, ok := m.checkboxRawLineAt(m.cursorRow); ok {
			m.pendingCheckboxRaw = rawLine
		}
	case "esc":
		// Esc dismisses an active selection first, same as most editors;
		// only navigates back once there's nothing left to deselect.
		if m.selActive {
			m.selActive = false
			break
		}
		m.back = true
	case "backspace":
		m.back = true
	}
	return m, nil
}

// moveCursorChar moves the block cursor one step in dir, without the
// heading-fold special case plain Left/Right have — extending a selection
// with Shift+Left/Right should always move the cursor, never collapse a
// heading. Shared by the Shift+Arrow cases in update().
func (m viewerModel) moveCursorChar(dir string, lines []string) viewerModel {
	switch dir {
	case "up":
		if m.cursorRow > 0 {
			m.cursorRow--
			m.cursorCol = clampCol(m.cursorCol, lines[m.cursorRow])
		}
	case "down":
		if m.cursorRow < len(lines)-1 {
			m.cursorRow++
			m.cursorCol = clampCol(m.cursorCol, lines[m.cursorRow])
		}
	case "left":
		if m.cursorCol > 0 {
			m.cursorCol--
		} else if m.cursorRow > 0 {
			m.cursorRow--
			m.cursorCol = xansi.StringWidth(lines[m.cursorRow])
		}
	case "right":
		curWidth := 0
		if m.cursorRow >= 0 && m.cursorRow < len(lines) {
			curWidth = xansi.StringWidth(lines[m.cursorRow])
		}
		if m.cursorCol < curWidth {
			m.cursorCol++
		} else if m.cursorRow < len(lines)-1 {
			m.cursorRow++
			m.cursorCol = 0
		}
	}
	return m
}

// selectedRawText returns the raw (unrendered) body text spanned by the
// active selection (#2). Selection endpoints live in rendered-line
// coordinates, but the clipboard payload must be the note's actual source
// text — not ANSI-styled output and not resolved wikilink alias text — so
// this reuses #22's rendered→raw map (rawLineAt) to find which raw lines the
// selection touches. That map is line-level, not character-exact (see its
// doc comment), so the copy is the full raw lines the selection's start and
// end rendered rows map to, not an exact character span within them.
func (m viewerModel) selectedRawText() string {
	if !m.selActive || m.note == nil {
		return ""
	}
	startRendered, endRendered := m.selAnchorRow, m.cursorRow
	if startRendered > endRendered {
		startRendered, endRendered = endRendered, startRendered
	}
	startRaw, endRaw := m.rawLineAt(startRendered), m.rawLineAt(endRendered)
	if startRaw > endRaw {
		startRaw, endRaw = endRaw, startRaw
	}
	rawBody := strings.Split(m.note.Body, "\n")
	if startRaw < 0 {
		startRaw = 0
	}
	if endRaw >= len(rawBody) {
		endRaw = len(rawBody) - 1
	}
	if startRaw > endRaw || startRaw >= len(rawBody) {
		return ""
	}
	return strings.Join(rawBody[startRaw:endRaw+1], "\n")
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
	if focused {
		bodyLines = m.withCursorOverlay(bodyLines)
	}

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
		right := fmt.Sprintf("%d%%", pct)
		avail := width - 2
		row := footerRow(avail, "Last saved: "+m.note.Updated.Format("15:04:05"), right)
		indicator = "\n" + lipgloss.NewStyle().Foreground(activeTheme.TextDim).Render(row)
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(activeTheme.Bg).
		Padding(0, 1).
		Render(sb.String() + indicator)
}

// footerRow lays out left-aligned and right-aligned text in a single row of
// the given width, degrading gracefully (shortening, then dropping, the left
// side) when there isn't room for both — a fixed-width footer that silently
// overflowed into a second line would desync the pane's rendered height from
// its bordered box height (see the editor's footerText for the same concern).
func footerRow(width int, left, right string) string {
	if gap := width - lipgloss.Width(left) - lipgloss.Width(right); gap >= 1 {
		return left + strings.Repeat(" ", gap) + right
	}
	// No room for both; drop left and right-align just the percentage.
	if gap := width - lipgloss.Width(right); gap >= 0 {
		return strings.Repeat(" ", gap) + right
	}
	return xansi.Truncate(right, width, "")
}

// withCursorOverlay highlights whatever interactive element the cursor is
// currently over (the whole rendered row for a link/checkbox, or every row
// of a code span), then draws the character-level block cursor on top.
// Operates on a copy — the cache in m.rendered is never mutated.
func (m viewerModel) withCursorOverlay(lines []string) []string {
	if m.cursorRow < 0 || m.cursorRow >= len(lines) {
		return lines
	}
	out := make([]string, len(lines))
	copy(out, lines)

	if m.selActive {
		// #2: selection highlight takes priority over the link/checkbox/code
		// highlight below. Whole rendered rows, not partial columns — same
		// line-level honesty as selectedRawText's copy, since a highlight
		// more precise than what gets copied would be misleading.
		start, end := m.selAnchorRow, m.cursorRow
		if start > end {
			start, end = end, start
		}
		for r := start; r <= end && r < len(out); r++ {
			if r >= 0 {
				out[r] = highlightPlain(lines[r], activeTheme.Accent, activeTheme.AccentFg)
			}
		}
	} else {
		_, isCheckbox := m.checkboxRawLineAt(m.cursorRow)
		switch cs, isCode := m.codeSpanAt(m.cursorRow); {
		case m.linkAtLine(m.cursorRow) != "", isCheckbox:
			out[m.cursorRow] = highlightPlain(lines[m.cursorRow], activeTheme.Accent, activeTheme.AccentFg)
		case isCode:
			for r := cs.startLine; r <= cs.endLine && r < len(out); r++ {
				if r >= 0 {
					out[r] = highlightPlain(lines[r], activeTheme.BlurredBg, activeTheme.TextPrimary)
				}
			}
		}
	}

	out[m.cursorRow] = overlayCursor(out[m.cursorRow], m.cursorCol)
	return out
}

// highlightPlain re-renders a rendered line's plain text (discarding its
// original per-token styling) in a single flat highlight color, the way a
// text selection overrides syntax coloring.
func highlightPlain(line string, bg, fg lipgloss.Color) string {
	plain := xansi.Strip(line)
	return lipgloss.NewStyle().Background(bg).Foreground(fg).Render(plain)
}

// overlayCursor draws a solid (non-blinking) block cursor over the character
// at display column col. The extracted single-width slice is stripped of its
// own ANSI codes before re-styling — glamour's output packs runs of
// zero-width "set-then-reset" segments at token boundaries, and a 1-column
// Cut can pull several of those in alongside the real glyph; rendering that
// raw would let an embedded reset cancel Reverse before the glyph ever
// prints, leaving no visible cursor at all (confirmed empirically).
func overlayCursor(line string, col int) string {
	width := xansi.StringWidth(line)
	if col < 0 {
		col = 0
	}
	if col > width {
		col = width
	}
	before := xansi.Cut(line, 0, col)
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	if col >= width {
		return before + cursorStyle.Render(" ")
	}
	ch := xansi.Strip(xansi.Cut(line, col, col+1))
	if ch == "" {
		ch = " "
	}
	after := xansi.Cut(line, col+1, width)
	return before + cursorStyle.Render(ch) + after
}

// copyToClipboardCmd copies content to the system clipboard via OSC 52, which
// works over SSH and inside tmux/zellij (unlike a local-only clipboard lib).
func copyToClipboardCmd(content string) tea.Cmd {
	return func() tea.Msg {
		osc52.New(content).WriteTo(os.Stdout)
		return nil
	}
}

// linkAtLine returns the note title linked on a given rendered line index, or "".
func (m viewerModel) linkAtLine(renderedLine int) string {
	if m.linkLines == nil {
		return ""
	}
	return m.linkLines[renderedLine]
}

// checkboxRawLineAt returns the raw body line index of the checkbox item on
// a given rendered line index, and whether one was found there.
func (m viewerModel) checkboxRawLineAt(renderedLine int) (int, bool) {
	if m.checkboxLines == nil {
		return 0, false
	}
	rawLine, ok := m.checkboxLines[renderedLine]
	return rawLine, ok
}

// headingRawLineAt returns the raw body line of the heading at the given
// rendered line, and whether one was found there (#20).
func (m viewerModel) headingRawLineAt(renderedLine int) (int, bool) {
	if m.headingLines == nil {
		return 0, false
	}
	rawLine, ok := m.headingLines[renderedLine]
	return rawLine, ok
}

// codeSpanAt returns the fenced code block whose rendered lines include the
// given line index, and whether one was found there.
func (m viewerModel) codeSpanAt(renderedLine int) (codeSpan, bool) {
	for _, cs := range m.codeSpans {
		if renderedLine >= cs.startLine && renderedLine <= cs.endLine {
			return cs, true
		}
	}
	return codeSpan{}, false
}

// parseTaskLine splits the text following a checkbox marker (e.g. " Task
// ✅ 2026-07-10 --> result") into its task text, completion date, and
// result. Any of the three may be absent, and date/result may appear in
// either order in the input — this is read-tolerant; formatTaskLine is what
// pins the canonical write order (text, then date, then result).
func parseTaskLine(rest string) (text, date, result string) {
	rest = strings.TrimLeft(rest, " ")
	if loc := taskDateRe.FindStringSubmatchIndex(rest); loc != nil {
		date = rest[loc[2]:loc[3]]
		rest = rest[:loc[0]] + rest[loc[1]:]
	}
	if idx := strings.Index(rest, taskResultSep); idx >= 0 {
		result = strings.TrimSpace(rest[idx+len(taskResultSep):])
		rest = rest[:idx]
	}
	text = strings.TrimRight(rest, " ")
	return text, date, result
}

// formatTaskLine rebuilds the text following a checkbox marker in the
// canonical order: text, then "✅ date" if set, then "--> result" if set.
// Leads with a single space to match "- [ ] text" spacing.
func formatTaskLine(text, date, result string) string {
	s := text
	if date != "" {
		s += " ✅ " + date
	}
	if result != "" {
		s += taskResultSep + result
	}
	return " " + s
}

// toggleCheckboxLine flips "[ ]" <-> "[x]" on the given raw line of body,
// stamping today's completion date on it when marking done and stripping
// the date when marking undone again (overwritten by repeated toggles, not
// accumulated). Any existing result (" --> ...") is preserved either way.
// ok is false if that line isn't a checkbox item (e.g. a stale raw-line
// index after the note was edited elsewhere).
func toggleCheckboxLine(body string, rawLine int) (string, bool) {
	lines := strings.Split(body, "\n")
	if rawLine < 0 || rawLine >= len(lines) {
		return body, false
	}
	m := checkboxLineRe.FindStringSubmatch(lines[rawLine])
	if m == nil {
		return body, false
	}
	nowDone := !(m[2] == "x" || m[2] == "X")
	mark := " "
	text, date, result := parseTaskLine(m[3])
	if nowDone {
		mark = "x"
		date = timeNow().Format("2006-01-02")
	} else {
		date = ""
	}
	lines[rawLine] = m[1] + "[" + mark + "]" + formatTaskLine(text, date, result)
	return strings.Join(lines, "\n"), true
}

// headingStyleConfig returns base with H2-H6 given distinct, non-literal
// styling. Glamour's built-in styles only set a literal "## "/"### " prefix
// for these levels (H1 alone gets a boxed, hash-free treatment), so without
// this they render as plain colored text with the hash marks still visible.
// It also brings fenced ``` code blocks to the same flat color/background
// swatch as inline `code`, instead of glamour's default per-token chroma
// syntax highlighting — the two rendered visibly differently otherwise.
func headingStyleConfig(base ansi.StyleConfig) ansi.StyleConfig {
	yes := true

	base.CodeBlock.Chroma = nil
	base.CodeBlock.Color = base.Code.Color
	base.CodeBlock.BackgroundColor = base.Code.BackgroundColor
	accent := string(activeTheme.Accent)
	sub := string(activeTheme.TextSecond)
	dim := string(activeTheme.TextDim)
	code := string(activeTheme.TextPrimary)
	codeBg := string(activeTheme.DropdownBg)

	base.H1.Color = &accent
	base.H1.BackgroundColor = nil
	base.Link.Color = &sub
	base.LinkText.Color = &sub
	base.Code.Color = &code
	base.Code.BackgroundColor = &codeBg
	base.CodeBlock.Color = &code
	base.CodeBlock.BackgroundColor = &codeBg

	base.H2.Prefix = ""
	base.H2.Bold = &yes
	base.H2.Underline = &yes
	base.H2.Color = &accent

	base.H3.Prefix = ""
	base.H3.Bold = &yes
	base.H3.Color = &accent

	base.H4.Prefix = ""
	base.H4.Bold = &yes
	base.H4.Italic = &yes
	base.H4.Color = &sub

	base.H5.Prefix = ""
	base.H5.Italic = &yes
	base.H5.Color = &sub

	base.H6.Prefix = ""
	base.H6.Italic = &yes
	base.H6.Color = &dim

	return base
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

// buildBodyMd tags checkboxes/code blocks/headings, substitutes wikilinks,
// and returns the body-only markdown for glamour, plus refs for recovering
// each interactive element's rendered position after rendering. folded is
// the note's current per-heading fold state (#20): lines hidden by a
// collapsed heading are dropped from the markdown handed to glamour
// entirely, so they never occupy a rendered line.
func buildBodyMd(n *vault.Note, titles map[string]bool, folded map[int]bool) (string, []linkRef, []checkboxRef, []codeRef, []headingRef, []lineRef) {
	hidden := hiddenLinesForFold(strings.Split(n.Body, "\n"), folded)
	annotated, checkboxRefs, codeRefs, headingRefs, lineRefs := annotateInteractive(n.Body, hidden, folded)
	body, linkRefs := substituteLinks(annotated, titles)
	return body, linkRefs, checkboxRefs, codeRefs, headingRefs, lineRefs
}

// checkboxRef records a task-list item found in the raw body, in parse order.
type checkboxRef struct {
	rawLine int // index into the raw body's lines
	checked bool
}

// codeRef records a fenced code block's raw content, in parse order.
type codeRef struct {
	content string // code only, fence markers excluded
}

// headingRef records a heading line found in the raw body, in parse order.
type headingRef struct {
	rawLine int // index into the raw body's lines
}

// lineRef records the start of an ordinary (plain-paragraph) block found in
// the raw body, in parse order (#22's general rendered→raw map).
type lineRef struct {
	rawLine int // index into the raw body's lines
}

// ordinaryLineSafe reports whether it's safe to prepend lineMark to a raw
// line without changing how goldmark parses it (#22). True only for a
// non-blank, unindented line with no leading syntax character of its own —
// a plain-paragraph line. Every proven-safe sentinel placement elsewhere in
// this file goes *after* a line's syntactic prefix (cbMark after "] ",
// headingMark after "# "); a plain paragraph has no such prefix to place it
// after, so it's marked at position 0 instead. Indented lines, list items,
// blockquotes, tables, fences, and thematic breaks are deliberately left
// unmarked — a leading sentinel would corrupt their block-type detection —
// and fall back to the nearest preceding mapped line via rawLineAt.
func ordinaryLineSafe(line string) bool {
	if line == "" || line != strings.TrimLeft(line, " \t") {
		return false
	}
	switch line[0] {
	case '#', '-', '*', '+', '>', '|', '`', '~':
		return false
	}
	j := 0
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}
	if j > 0 && j < len(line) && (line[j] == '.' || line[j] == ')') {
		return false // ordered-list marker, e.g. "1. " or "2) "
	}
	return true
}

// headingLevel returns a markdown heading line's level (1-6), the count of
// leading '#' characters. Callers must confirm headingLineRe matched first.
func headingLevel(line string) int {
	n := 0
	for n < len(line) && n < 6 && line[n] == '#' {
		n++
	}
	return n
}

// hiddenLinesForFold returns the set of raw body-line indices to hide given
// the current per-heading fold state (#20). A collapsed heading hides every
// line until the next heading of the same or higher level — including any
// nested headings' own lines, whose individual fold state doesn't matter
// while an ancestor is collapsed. Returns nil (not an empty map) when
// nothing is folded, so callers can skip the hidden-line bookkeeping
// entirely in the common case.
func hiddenLinesForFold(lines []string, folded map[int]bool) map[int]bool {
	if len(folded) == 0 {
		return nil
	}
	hidden := map[int]bool{}
	hiding := false
	hidingLevel := 0
	for i, line := range lines {
		if headingLineRe.MatchString(line) {
			level := headingLevel(line)
			if hiding {
				if level <= hidingLevel {
					hiding = false
				} else {
					hidden[i] = true
					continue
				}
			}
			if folded[i] {
				hiding = true
				hidingLevel = level
			}
			continue
		}
		if hiding {
			hidden[i] = true
		}
	}
	return hidden
}

// fenceLineRe matches a fenced-code-block delimiter line (``` or ~~~ fences
// are not distinguished — only backtick fences are used in this app).
var fenceLineRe = regexp.MustCompile("^\\s*```")

// PUA sentinels for checkbox/code-block position tracking (see the link
// sentinels above for the same technique). Placed after "] " rather than
// wrapping the checkbox brackets themselves, and inside — not around — the
// fenced fence delimiters, because goldmark's task-list and code-fence
// detection only recognizes the literal syntax at the very start of the
// line; sentinels placed there would silently fall back to a plain bullet
// (verified empirically against glamour's task-list rendering).
const (
	cbMark      = "" // checkbox row marker
	codeOpen    = "" // first content line of a fenced code block
	codeClose   = "" // last content line of a fenced code block
	headingMark = "" // heading line marker, for fold click/keyboard targeting
	lineMark    = "" // ordinary-paragraph-block start marker (#22)
)

// annotateInteractive scans the raw body line-by-line for checkbox items and
// fenced code blocks, tagging them with invisible PUA sentinels so their
// rendered position can be recovered after glamour renders the markdown —
// reflow/word-wrap means rendered lines don't map 1:1 to raw lines, but a
// sentinel travels through rendering as literal text and can be found again
// (verified empirically, including inside chroma-highlighted code). Must run
// before substituteLinks, which uses a different sentinel range.
func annotateInteractive(body string, hidden map[int]bool, folded map[int]bool) (string, []checkboxRef, []codeRef, []headingRef, []lineRef) {
	lines := strings.Split(body, "\n")
	var checkboxRefs []checkboxRef
	var codeRefs []codeRef
	var headingRefs []headingRef
	var lineRefs []lineRef

	inFence := false
	fenceStart := 0

	i := 0
	for i < len(lines) {
		if hidden[i] {
			i++
			continue
		}
		line := lines[i]

		if fenceLineRe.MatchString(line) {
			if !inFence {
				inFence = true
				fenceStart = i + 1
			} else {
				inFence = false
				content := lines[fenceStart:i]
				if len(content) > 0 {
					codeRefs = append(codeRefs, codeRef{content: strings.Join(content, "\n")})
					lines[fenceStart] = codeOpen + lines[fenceStart]
					lines[i-1] = lines[i-1] + codeClose
				}
			}
			i++
			continue
		}
		if inFence {
			i++
			continue
		}
		if headingLineRe.MatchString(line) {
			level := headingLevel(line)
			hashes := strings.Repeat("#", level)
			rest := strings.TrimPrefix(strings.TrimPrefix(line, hashes), " ")
			glyph := "▼ "
			if folded[i] {
				glyph = "▶ "
			}
			lines[i] = hashes + " " + headingMark + glyph + rest
			headingRefs = append(headingRefs, headingRef{rawLine: i})
			i++
			continue
		}
		if checkboxLineRe.MatchString(line) {
			// A "list" is a maximal run of consecutive checkbox lines (#12):
			// gather it, then stable-partition unfinished-before-finished for
			// DISPLAY, while each ref keeps its true original raw line index
			// (idx) so toggling still edits the right line after the reorder.
			// Never reorder across a non-checkbox line into another block.
			blockStart := i
			type cbLine struct {
				idx     int
				checked bool
				text    string
			}
			var block []cbLine
			for i < len(lines) {
				m := checkboxLineRe.FindStringSubmatch(lines[i])
				if m == nil {
					break
				}
				block = append(block, cbLine{
					idx:     i,
					checked: m[2] == "x" || m[2] == "X",
					text:    m[1] + "[" + m[2] + "]" + cbMark + m[3],
				})
				i++
			}
			var ordered []cbLine
			for _, b := range block {
				if !b.checked {
					ordered = append(ordered, b)
				}
			}
			for _, b := range block {
				if b.checked {
					ordered = append(ordered, b)
				}
			}
			for j, b := range ordered {
				lines[blockStart+j] = b.text
				checkboxRefs = append(checkboxRefs, checkboxRef{rawLine: b.idx, checked: b.checked})
			}
			continue
		}
		if ordinaryLineSafe(line) {
			// #22: mark only the block's first raw line — glamour merges
			// consecutive plain-paragraph raw lines into one reflowed block,
			// so per-source-line rendered positions inside it aren't
			// recoverable anyway (see ordinaryLineSafe's doc comment).
			// Continuation lines fall back to this anchor via rawLineAt.
			lines[i] = lineMark + line
			lineRefs = append(lineRefs, lineRef{rawLine: i})
			i++
			for i < len(lines) && !hidden[i] && ordinaryLineSafe(lines[i]) {
				i++
			}
			continue
		}
		i++
	}
	if len(hidden) > 0 {
		out := make([]string, 0, len(lines))
		for i, l := range lines {
			if hidden[i] {
				continue
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n"), checkboxRefs, codeRefs, headingRefs, lineRefs
	}
	return strings.Join(lines, "\n"), checkboxRefs, codeRefs, headingRefs, lineRefs
}

// processCheckboxesAndCode strips checkbox/code sentinels from the rendered
// body, building rendered-line maps back to the raw refs annotateInteractive
// found. Sentinels are matched to refs in left-to-right parse order, the
// same convention processRenderedLinks uses for links.
func processCheckboxesAndCode(rendered string, checkboxRefs []checkboxRef, codeRefs []codeRef, headingRefs []headingRef, lineRefs []lineRef) (string, map[int]int, []codeSpan, map[int]int, map[int]int) {
	lines := strings.Split(rendered, "\n")
	checkboxLines := make(map[int]int)
	headingLines := make(map[int]int)
	rawLines := make(map[int]int)
	var spans []codeSpan

	cbIdx, codeIdx, headIdx, lineRefIdx := 0, 0, 0, 0
	openCodeLine := -1
	// curRaw is the #22 forward-fill pointer: the raw line of the most
	// recent heading/checkbox/paragraph anchor seen so far, carried onto
	// every rendered line (including word-wrapped continuations and
	// glamour's own block-spacing blanks) until the next anchor updates it.
	curRaw := -1
	// dimUntilNextTask keeps dimming a finished task's word-wrapped
	// continuation lines (#17): the cbMark sentinel only lands on a task's
	// first rendered line, so without this a long done task read as grey
	// on line one and normal-colored on every wrapped line after it. The
	// run ends at a blank line, the next task (any cbMark line), a heading,
	// or a code fence boundary — never by indentation, since glamour
	// indents nested list items the same as wrapped task text.
	dimUntilNextTask := false

	for lineIdx, line := range lines {
		hasCodeFenceMark := strings.Contains(line, codeOpen) || strings.Contains(line, codeClose)
		hasHeadingMark := strings.Contains(line, headingMark)

		if idx := strings.Index(line, headingMark); idx != -1 {
			if headIdx < len(headingRefs) {
				headingLines[lineIdx] = headingRefs[headIdx].rawLine
				curRaw = headingRefs[headIdx].rawLine
				headIdx++
			}
			line = line[:idx] + line[idx+len(headingMark):]
		}

		if idx := strings.Index(line, lineMark); idx != -1 {
			if lineRefIdx < len(lineRefs) {
				curRaw = lineRefs[lineRefIdx].rawLine
				lineRefIdx++
			}
			line = line[:idx] + line[idx+len(lineMark):]
		}

		if idx := strings.Index(line, cbMark); idx != -1 {
			finished := false
			if cbIdx < len(checkboxRefs) {
				checkboxLines[lineIdx] = checkboxRefs[cbIdx].rawLine
				curRaw = checkboxRefs[cbIdx].rawLine
				finished = checkboxRefs[cbIdx].checked
				cbIdx++
			}
			line = line[:idx] + line[idx+len(cbMark):]
			if finished {
				// #12: finished tasks get a permanent muted/table-like
				// treatment instead of glamour's normal task-item styling,
				// so the sunk-to-the-bottom group also reads as visually
				// secondary. Re-tints in place (like the cursor's
				// highlightPlain overlay) rather than rebuilding the line
				// from scratch, so an already-rendered result wikilink
				// (#11) keeps its alias text and stays clickable via the
				// existing linkLines mapping — only the color changes.
				line = highlightPlain(line, activeTheme.Bg, activeTheme.TextDim)
			}
			dimUntilNextTask = finished
		} else if dimUntilNextTask {
			if strings.TrimSpace(line) == "" || hasCodeFenceMark || hasHeadingMark {
				dimUntilNextTask = false
			} else {
				line = highlightPlain(line, activeTheme.Bg, activeTheme.TextDim)
			}
		}
		if idx := strings.Index(line, codeOpen); idx != -1 {
			openCodeLine = lineIdx
			line = line[:idx] + line[idx+len(codeOpen):]
		}
		if idx := strings.Index(line, codeClose); idx != -1 {
			line = line[:idx] + line[idx+len(codeClose):]
			if openCodeLine != -1 && codeIdx < len(codeRefs) {
				spans = append(spans, codeSpan{startLine: openCodeLine, endLine: lineIdx, content: codeRefs[codeIdx].content})
				codeIdx++
			}
			openCodeLine = -1
		}
		if curRaw >= 0 {
			rawLines[lineIdx] = curRaw
		}
		lines[lineIdx] = line
	}
	return strings.Join(lines, "\n"), checkboxLines, spans, headingLines, rawLines
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
