package tui

import (
	"fmt"
	"strings"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Section landing pages ─────────────────────────────────────────────────────

type sectionLandingPane struct {
	state       vault.NoteState
	isTemplates bool
	notes       []*vault.Note
	count       int
}

func (sl sectionLandingPane) render(width, height int) string {
	t := activeTheme
	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	muted := lipgloss.NewStyle().Foreground(t.TextSecond)
	dim := lipgloss.NewStyle().Foreground(t.TextDim)
	noteStyle := lipgloss.NewStyle().Foreground(t.TextMuted)

	var b strings.Builder

	var sectionName string
	if sl.isTemplates {
		sectionName = "#templates"
	} else {
		sectionName = capitalize(string(sl.state))
	}

	countStr := fmt.Sprintf("%d notes", sl.count)
	if sl.count == 1 {
		countStr = "1 note"
	}

	header := sectionName
	pad := width - len([]rune(header)) - len([]rune(countStr))
	if pad < 1 {
		pad = 1
	}
	b.WriteString(heading.Render(header) + strings.Repeat(" ", pad) + muted.Render(countStr))
	b.WriteString("\n\n")

	if desc := sectionDescription(sl.state, sl.isTemplates); desc != "" {
		b.WriteString(dim.Render(desc))
		b.WriteString("\n\n")
	}

	if len(sl.notes) == 0 {
		b.WriteString(dim.Render("No notes here yet."))
		if !sl.isTemplates {
			b.WriteString("\n" + dim.Render(`:new "Title" to create one.`))
		}
	} else {
		limit := 15
		if len(sl.notes) < limit {
			limit = len(sl.notes)
		}
		for _, n := range sl.notes[:limit] {
			line := "  · " + truncate(n.Title, width-5)
			b.WriteString(noteStyle.Render(line) + "\n")
		}
		if len(sl.notes) > 15 {
			b.WriteString(dim.Render(fmt.Sprintf("  … and %d more", len(sl.notes)-15)) + "\n")
		}
	}

	return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(b.String())
}

func sectionDescription(state vault.NoteState, isTemplates bool) string {
	if isTemplates {
		return "Reusable note templates. Use :insert <template> to apply to an open note."
	}
	switch state {
	case vault.StateInbox:
		return "New notes land here. Promote to Projects, Areas, or Research when ready."
	case vault.StateAreas:
		return "Ongoing responsibilities and areas of life to maintain over time."
	case vault.StateResearch:
		return "Notes for learning, exploration, and reference material."
	case vault.StateArchive:
		return "Inactive and completed notes. Final resting place for finished work."
	}
	return ""
}

// ── Projects overview ─────────────────────────────────────────────────────────

// renderProjectsOverview renders a summary of all active projects.
func renderProjectsOverview(projects []*vault.Project, allNotes []*vault.Note, width, height int) string {
	t := activeTheme
	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	muted := lipgloss.NewStyle().Foreground(t.TextSecond)
	dim := lipgloss.NewStyle().Foreground(t.TextDim)
	noteIndent := lipgloss.NewStyle().Foreground(t.TextMuted)

	var b strings.Builder
	b.WriteString(heading.Render(fmt.Sprintf("Active Projects  %d/%d", len(projects), vault.MaxProjects)))
	b.WriteString("\n\n")

	if len(projects) == 0 {
		b.WriteString(dim.Render("No active projects. Assign a note to a project with Shift+P."))
		return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(b.String())
	}

	for _, p := range projects {
		// Gather notes for this project.
		var pNotes []*vault.Note
		for _, n := range allNotes {
			if n.Project == p.Name && n.State == vault.StateProjects {
				pNotes = append(pNotes, n)
			}
		}
		noteCount := len(pNotes)
		countLabel := "notes"
		if noteCount == 1 {
			countLabel = "note"
		}

		header := fmt.Sprintf("◆ %s", p.Name)
		countStr := fmt.Sprintf("%d %s", noteCount, countLabel)
		// Pad header to width, put count on right.
		pad := width - len([]rune(header)) - len([]rune(countStr))
		if pad < 1 {
			pad = 1
		}
		b.WriteString(heading.Render(header) + strings.Repeat(" ", pad) + muted.Render(countStr))
		b.WriteString("\n")

		for _, n := range pNotes {
			line := "  · " + truncate(n.Title, width-5)
			b.WriteString(noteIndent.Render(line))
			b.WriteString("\n")
		}
		if noteCount == 0 {
			b.WriteString(dim.Render("  (no notes)"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(b.String())
}

// ── Project detail pane ───────────────────────────────────────────────────────

type projectDetailPane struct {
	project       *vault.Project
	notes         []*vault.Note
	scrollOffset  int
	editingBridge bool
	bridgeInput   string
	bridgeDone    bool   // a bridge entry was submitted; caller should save & clear
	bridgeMessage string // the submitted message

	// Task list (#19): a filtered, per-project version of the :tasks
	// overview. Focus is either the bridge input (editingBridge, entered
	// with e/i, left with Esc or a submitted Enter — unchanged) or the task
	// list (the default). Tab was considered as an additional focus switch
	// but collides with the global sidebar/main pane-focus binding once
	// editingBridge is false, so it is not used here — see #19's closing
	// notes. taskCursorRow indexes into taskRows and only ever rests on a
	// row with task != nil (header/spacer rows are skipped when moving,
	// same convention as taskOverviewRow).
	taskRows      []taskOverviewRow
	taskCursorRow int
	pendingToggle bool   // caller should toggle the task under the cursor, then clear this
	pendingOpen   string // caller should open this note title, then clear this
}

func newProjectDetailPane(p *vault.Project, notes []*vault.Note) projectDetailPane {
	rows := projectTaskRows(notes)
	cursor := 0
	for i, r := range rows {
		if r.task != nil {
			cursor = i
			break
		}
	}
	return projectDetailPane{project: p, notes: notes, taskRows: rows, taskCursorRow: cursor}
}

// taskRowIndices returns the indices of rows in pd.taskRows that carry an
// actual task, in order — the only rows the cursor may rest on.
func taskRowIndices(rows []taskOverviewRow) []int {
	var idx []int
	for i, r := range rows {
		if r.task != nil {
			idx = append(idx, i)
		}
	}
	return idx
}

// moveTaskCursor steps the cursor by delta among task-only rows, clamping
// at both ends, skipping note-header/spacer rows.
func (pd projectDetailPane) moveTaskCursor(delta int) projectDetailPane {
	idx := taskRowIndices(pd.taskRows)
	if len(idx) == 0 {
		return pd
	}
	pos := 0
	for i, v := range idx {
		if v == pd.taskCursorRow {
			pos = i
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(idx) {
		pos = len(idx) - 1
	}
	pd.taskCursorRow = idx[pos]
	return pd
}

func (pd projectDetailPane) update(msg tea.KeyMsg) projectDetailPane {
	if pd.editingBridge {
		switch msg.String() {
		case "enter":
			if strings.TrimSpace(pd.bridgeInput) != "" {
				pd.bridgeDone = true
				pd.bridgeMessage = strings.TrimSpace(pd.bridgeInput)
				pd.bridgeInput = ""
				pd.editingBridge = false
			}
		case "esc":
			pd.bridgeInput = ""
			pd.editingBridge = false
		case "backspace", "ctrl+h":
			runes := []rune(pd.bridgeInput)
			if len(runes) > 0 {
				pd.bridgeInput = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			pd.bridgeInput = ""
		default:
			if len(msg.Runes) > 0 {
				pd.bridgeInput += string(msg.Runes)
			}
		}
		return pd
	}

	hasTasks := len(taskRowIndices(pd.taskRows)) > 0
	switch msg.String() {
	case "j", "down":
		if hasTasks {
			pd = pd.moveTaskCursor(1)
		} else {
			pd.scrollOffset++
		}
	case "k", "up":
		if hasTasks {
			pd = pd.moveTaskCursor(-1)
		} else if pd.scrollOffset > 0 {
			pd.scrollOffset--
		}
	case "e", "i":
		pd.editingBridge = true
		pd.bridgeInput = ""
	case " ":
		if hasTasks {
			pd.pendingToggle = true
		}
	case "enter":
		if pd.taskCursorRow >= 0 && pd.taskCursorRow < len(pd.taskRows) {
			if row := pd.taskRows[pd.taskCursorRow]; row.task != nil {
				pd.pendingOpen = row.task.note.Title
			}
		}
	}
	return pd
}

func (pd projectDetailPane) render(width, height int) string {
	t := activeTheme
	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	muted := lipgloss.NewStyle().Foreground(t.TextSecond)
	dim := lipgloss.NewStyle().Foreground(t.TextDim)
	noteStyle := lipgloss.NewStyle().Foreground(t.TextMuted)
	sepStyle := lipgloss.NewStyle().Foreground(t.TextDim)
	h2Style := lipgloss.NewStyle().Foreground(t.TextSecond)
	doneStyle := lipgloss.NewStyle().Foreground(t.TextDim)
	textStyle := lipgloss.NewStyle().Foreground(t.TextPrimary)
	cursorStyle := lipgloss.NewStyle().Background(t.Accent).Foreground(t.AccentFg)

	sep := func(label string) string {
		line := "── " + label + " "
		remaining := width - len([]rune(line))
		if remaining > 0 {
			line += strings.Repeat("─", remaining)
		}
		return sepStyle.Render(line)
	}

	var lines []string

	if pd.project == nil {
		return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).
			Render(dim.Render("No project selected."))
	}

	p := pd.project

	lines = append(lines, heading.Render("◆ "+p.Name))
	lines = append(lines, dim.Render("  Created: "+p.Created.Format("2006-01-02")))
	lines = append(lines, "")

	// Notes section
	lines = append(lines, sep("Notes"))
	if len(pd.notes) == 0 {
		lines = append(lines, dim.Render("  (no notes assigned)"))
	} else {
		for _, n := range pd.notes {
			lines = append(lines, noteStyle.Render("  · "+truncate(n.Title, width-5)))
		}
	}
	lines = append(lines, "")

	// History section
	lines = append(lines, sep("History"))
	if len(p.History) == 0 {
		lines = append(lines, dim.Render("  (no entries yet)"))
	} else {
		// Show history newest-first
		for i := len(p.History) - 1; i >= 0; i-- {
			entry := p.History[i]
			ts := entry.Timestamp.Format("2006-01-02 15:04")
			var text string
			switch entry.Kind {
			case vault.HistoryKindAttached:
				text = muted.Render(ts) + "  " + dim.Render("attached") + "  " + entry.NoteTitle
			case vault.HistoryKindDetached:
				text = muted.Render(ts) + "  " + dim.Render("detached") + "  " + entry.NoteTitle
			case vault.HistoryKindNote:
				text = muted.Render(ts) + "  " + entry.Message
			}
			lines = append(lines, "  "+text)
		}
	}
	lines = append(lines, "")

	// Tasks section (#19): a per-project filtered view of :tasks, grouped
	// by note. taskLineAt records which line in `lines` each row landed on,
	// so the cursor can be highlighted and auto-scroll can follow it.
	lines = append(lines, sep("Tasks"))
	taskLineAt := make([]int, len(pd.taskRows))
	if len(pd.taskRows) == 0 {
		lines = append(lines, dim.Render("  (no tasks)"))
	} else {
		for i, row := range pd.taskRows {
			var content string
			switch {
			case row.fileNote != nil:
				content = h2Style.Render("  " + row.fileNote.Title)
			case row.task != nil:
				box := "[ ]"
				style := textStyle
				if row.task.done {
					box = "[x]"
					style = doneStyle
				}
				text := row.task.text
				if row.task.date != "" {
					text += " ✅ " + row.task.date
				}
				if row.task.result != "" {
					text += " --> " + row.task.result
				}
				content = style.Render(fmt.Sprintf("    %s %s", box, truncate(text, max(1, width-8))))
			}
			if i == pd.taskCursorRow && !pd.editingBridge {
				pad := width - lipgloss.Width(content)
				if pad < 0 {
					pad = 0
				}
				content = cursorStyle.Render(content + strings.Repeat(" ", pad))
			}
			taskLineAt[i] = len(lines)
			lines = append(lines, content)
		}
	}
	lines = append(lines, "")

	// Hemingway bridge input area
	if pd.editingBridge {
		inputLine := "  > " + pd.bridgeInput + "█"
		lines = append(lines, sep("Add entry"))
		lines = append(lines, lipgloss.NewStyle().Foreground(t.AccentFg).Render(inputLine))
		lines = append(lines, dim.Render("  Enter to save · Esc to cancel"))
	} else {
		lines = append(lines, dim.Render("  Press e to add a history entry"))
	}

	// Apply scroll: auto-follow the task cursor when the task list has
	// focus, otherwise fall back to the plain scrollOffset (used only when
	// there are no tasks to select, see update()'s hasTasks branch).
	scrollOffset := pd.scrollOffset
	if idx := taskRowIndices(pd.taskRows); len(idx) > 0 && !pd.editingBridge {
		cursorLine := taskLineAt[pd.taskCursorRow]
		if cursorLine < scrollOffset {
			scrollOffset = cursorLine
		} else if cursorLine >= scrollOffset+height {
			scrollOffset = cursorLine - height + 1
		}
	}
	visible := lines
	if scrollOffset > 0 && scrollOffset < len(lines) {
		visible = lines[scrollOffset:]
	}
	if len(visible) > height {
		visible = visible[:height]
	}

	content := strings.Join(visible, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(content)
}
