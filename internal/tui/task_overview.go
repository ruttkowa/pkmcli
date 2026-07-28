package tui

import (
	"fmt"
	"sort"
	"strings"

	"pkm/internal/gitlab"
	"pkm/internal/vault"

	"github.com/charmbracelet/lipgloss"
)

// taskEntry is one task-list line found while scanning the vault, carrying
// enough to render it and reopen its source note.
type taskEntry struct {
	note    *vault.Note
	rawLine int
	text    string
	done    bool
	date    string
	result  string
}

// taskOverviewRow is one row of the flattened Task Overview: exactly one of
// projectHeader/fileNote/task is set, or none for a blank spacer row. Kept
// flat (rather than nested groups) so a row index doubles as the cursor
// position, independent of rendering width — no word-wrap is applied to
// task lines, so this mapping never needs to change with pane width.
type taskOverviewRow struct {
	projectHeader string      // H1: project name
	fileNote      *vault.Note // H2: note title header
	task          *taskEntry  // an actual task line
	issue         *gitlab.Issue
	issueProject  string
}

// notesWithTasks scans a note's body for checkbox lines and returns them as
// taskEntry values in the order they appear in the file.
func notesWithTasks(n *vault.Note) []taskEntry {
	var entries []taskEntry
	lines := strings.Split(n.Body, "\n")
	for i, line := range lines {
		m := checkboxLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text, date, result := parseTaskLine(m[3])
		entries = append(entries, taskEntry{
			note:    n,
			rawLine: i,
			text:    text,
			done:    m[2] == "x" || m[2] == "X",
			date:    date,
			result:  result,
		})
	}
	return entries
}

// projectTaskRows flattens a set of notes' tasks into rows grouped by note
// (H2 header per note, sorted by title, followed by its tasks in file
// order), skipping notes with no tasks. The shared grouping core used both
// by the vault-wide Task Overview (per project/unassigned group) and the
// Project Detail pane's filtered task list (#19).
func projectTaskRows(notes []*vault.Note) []taskOverviewRow {
	var withTasks []*vault.Note
	for _, n := range notes {
		if len(notesWithTasks(n)) > 0 {
			withTasks = append(withTasks, n)
		}
	}
	sort.Slice(withTasks, func(i, j int) bool { return withTasks[i].Title < withTasks[j].Title })

	var rows []taskOverviewRow
	for _, n := range withTasks {
		rows = append(rows, taskOverviewRow{fileNote: n})
		for _, te := range notesWithTasks(n) {
			te := te
			rows = append(rows, taskOverviewRow{task: &te})
		}
	}
	return rows
}

// buildTaskOverviewRows scans every note in the vault for tasks and
// flattens them into the Task Overview's row order: each active project
// (in sidebar order) with task-bearing notes gets an H1, each such note
// gets an H2 (notes sorted by title) followed by its tasks in file order;
// then a trailing "Unassigned" group for every other task-bearing note
// (also sorted by title), grouped by file the same way.
func buildTaskOverviewRows(v *vault.Vault) []taskOverviewRow {
	allNotes, err := v.ListAll()
	if err != nil {
		return nil
	}

	byProject := map[string][]*vault.Note{}
	var unassigned []*vault.Note
	for _, n := range allNotes {
		if len(notesWithTasks(n)) == 0 {
			continue
		}
		if n.State == vault.StateProjects && n.Project != "" {
			byProject[n.Project] = append(byProject[n.Project], n)
		} else {
			unassigned = append(unassigned, n)
		}
	}

	var rows []taskOverviewRow
	for _, p := range v.Projects.ListActive() {
		notes := byProject[p.Name]
		if len(notes) == 0 {
			continue
		}
		rows = append(rows, taskOverviewRow{projectHeader: p.Name})
		rows = append(rows, projectTaskRows(notes)...)
	}

	if len(unassigned) > 0 {
		rows = append(rows, taskOverviewRow{projectHeader: "Unassigned"})
		rows = append(rows, projectTaskRows(unassigned)...)
	}

	return rows
}

// renderTaskOverview formats the flattened rows into a scrollable pane,
// highlighting cursorRow the same way the sidebar highlights its cursor.
func renderTaskOverview(rows []taskOverviewRow, width, height, scrollOff, cursorRow int, focused bool) string {
	t := activeTheme
	h1Style := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	h2Style := lipgloss.NewStyle().Foreground(t.TextSecond)
	doneStyle := lipgloss.NewStyle().Foreground(t.TextDim)
	textStyle := lipgloss.NewStyle().Foreground(t.TextPrimary)
	cursorStyle := lipgloss.NewStyle().Background(t.Accent).Foreground(t.AccentFg)

	if len(rows) == 0 {
		empty := lipgloss.NewStyle().Foreground(t.TextDim).Render("No tasks found in this vault.")
		return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(empty)
	}

	lines := make([]string, len(rows))
	for i, row := range rows {
		var content string
		switch {
		case row.projectHeader != "":
			content = h1Style.Render(row.projectHeader)
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
		case row.issue != nil:
			text := fmt.Sprintf("    #%d %s", row.issue.IID, row.issue.Title)
			if len(row.issue.Labels) > 0 {
				text += "  [" + strings.Join(row.issue.Labels, ", ") + "]"
			}
			content = textStyle.Render(truncate(text, max(1, width)))
		}

		if i == cursorRow {
			pad := width - lipgloss.Width(content)
			if pad < 0 {
				pad = 0
			}
			content = cursorStyle.Render(content + strings.Repeat(" ", pad))
			if !focused {
				content = lipgloss.NewStyle().Background(t.BlurredBg).Foreground(t.TextPrimary).Render(content)
			}
		}
		lines[i] = content
	}

	if scrollOff > len(lines) {
		scrollOff = len(lines)
	}
	if scrollOff < 0 {
		scrollOff = 0
	}
	end := scrollOff + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scrollOff:end]

	blank := lipgloss.NewStyle().Width(width).Background(t.Bg).Render("")
	for len(visible) < height {
		visible = append(visible, blank)
	}

	return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(strings.Join(visible, "\n"))
}
