package tui

import (
	"fmt"
	"strconv"
	"strings"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sidebarItem is one row in the flat navigation list.
type sidebarItem struct {
	isSection      bool
	isTemplates    bool // true for the virtual #templates section
	isTasks        bool // true for the virtual Tasks nav row
	isProjectEntry bool // project folder row (collapsible)
	isProjectNote  bool // note row under a project folder
	state          vault.NoteState
	note           *vault.Note
	project        *vault.Project
}

type sidebarModel struct {
	index           *index.Index
	vault           *vault.Vault
	activeState     vault.NoteState
	cursor          int
	counts          map[vault.NoteState]int
	expanded        map[vault.NoteState]bool
	notesByState    map[vault.NoteState][]*vault.Note
	selected        bool
	selectedNote    *vault.Note    // non-nil when a note title was chosen
	selectedProject *vault.Project // non-nil when a project entry was chosen
	selectedTasks   bool           // true when the Tasks row was chosen

	templatesExpanded bool
	templatesActive   bool
	templateNotes     []*vault.Note
	templateCount     int

	tasksActive bool // true when the Task Overview is the active main-pane view

	// visibility, driven by AppConfig (#14) — synced in New(), applyConfigItem,
	// and applyConfig, never read directly from cfg so items()'s three call
	// sites (render, keyboard update, mouse click) don't need cfg threaded in.
	showTasksNav     bool
	showTemplatesNav bool

	// project folder state (within the expanded Projects section)
	activeProjectName  string                   // name of the focused project folder/note
	projectsActive     bool                     // true when a project folder/note is the active view
	expandedProjects   map[string]bool          // which project folders are open
	projectNotesByName map[string][]*vault.Note // cached notes per project
}

func newSidebar(idx *index.Index, v *vault.Vault) sidebarModel {
	return sidebarModel{
		index:              idx,
		vault:              v,
		activeState:        vault.StateInbox,
		counts:             map[vault.NoteState]int{},
		expanded:           map[vault.NoteState]bool{},
		notesByState:       map[vault.NoteState][]*vault.Note{},
		expandedProjects:   map[string]bool{},
		projectNotesByName: map[string][]*vault.Note{},
	}
}

func (s sidebarModel) init() tea.Cmd {
	return func() tea.Msg {
		counts, _ := s.index.CountByState()
		templates, _ := s.vault.ListByTag("template")
		return countsRefreshedMsg{counts: counts, templateCount: len(templates)}
	}
}

func (s sidebarModel) withCounts(counts map[vault.NoteState]int) sidebarModel {
	s.counts = counts
	return s
}

// refreshNotes reloads note lists for all currently expanded sections and templates.
func (s sidebarModel) refreshNotes() sidebarModel {
	for state, expanded := range s.expanded {
		if expanded && state != vault.StateProjects {
			notes, _ := s.vault.ListByState(state)
			s.notesByState[state] = notes
		}
	}
	// Refresh expanded project folders.
	if s.expanded[vault.StateProjects] {
		allNotes, _ := s.vault.ListAll()
		for name, expanded := range s.expandedProjects {
			if expanded {
				var pNotes []*vault.Note
				for _, n := range allNotes {
					if n.Project == name && n.State == vault.StateProjects {
						pNotes = append(pNotes, n)
					}
				}
				s.projectNotesByName[name] = pNotes
			}
		}
	}
	notes, _ := s.vault.ListByTag("template")
	s.templateCount = len(notes)
	if s.templatesExpanded {
		s.templateNotes = notes
	}
	return s
}

// items returns the flat ordered list of all visible sidebar rows.
func (s sidebarModel) items() []sidebarItem {
	var out []sidebarItem
	for _, state := range vault.AllStates {
		out = append(out, sidebarItem{isSection: true, state: state})
		if s.expanded[state] {
			if state == vault.StateProjects {
				// Show project folders, each optionally expanded to show their notes.
				for _, p := range s.vault.Projects.ListActive() {
					out = append(out, sidebarItem{isProjectEntry: true, project: p})
					if s.expandedProjects[p.Name] {
						for _, n := range s.projectNotesByName[p.Name] {
							out = append(out, sidebarItem{isProjectNote: true, project: p, note: n})
						}
					}
				}
			} else {
				for _, n := range s.notesByState[state] {
					out = append(out, sidebarItem{state: state, note: n})
				}
			}
		}
	}
	// Virtual templates section
	if s.showTemplatesNav {
		out = append(out, sidebarItem{isSection: true, isTemplates: true})
		if s.templatesExpanded {
			for _, n := range s.templateNotes {
				out = append(out, sidebarItem{isTemplates: true, note: n})
			}
		}
	}
	// Virtual Tasks nav row — no children, no expand/collapse.
	if s.showTasksNav {
		out = append(out, sidebarItem{isSection: true, isTasks: true})
	}
	return out
}

// clampCursor keeps the cursor within bounds after items() shrinks — e.g.
// when a config toggle hides the Tasks or Templates row.
func (s *sidebarModel) clampCursor() {
	n := len(s.items())
	if s.cursor >= n {
		s.cursor = n - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// refreshNotesPreservingCursor reloads expanded rows without letting a
// disappearing row shift the numeric cursor onto an unrelated section. This
// matters most after deleting the selected note: the next row at the old
// index may be Archive even though the user is still working in Areas.
func (s *sidebarModel) refreshNotesPreservingCursor() {
	oldItems := s.items()
	var old sidebarItem
	hadOld := s.cursor >= 0 && s.cursor < len(oldItems)
	if hadOld {
		old = oldItems[s.cursor]
	}

	*s = s.refreshNotes()
	if !hadOld {
		s.clampCursor()
		return
	}

	newItems := s.items()
	for i, item := range newItems {
		if sameSidebarItem(old, item) {
			s.cursor = i
			return
		}
	}

	// The selected note was removed. Land on its containing project/section
	// instead of retaining a stale flat-list index.
	for i, item := range newItems {
		switch {
		case old.isProjectNote && item.isProjectEntry && old.project != nil &&
			item.project != nil && item.project.Name == old.project.Name:
			s.cursor = i
			return
		case old.isTemplates && old.note != nil && item.isSection && item.isTemplates:
			s.cursor = i
			return
		case old.note != nil && !old.isProjectNote && !old.isTemplates &&
			item.isSection && !item.isTemplates && !item.isTasks && item.state == old.state:
			s.cursor = i
			return
		}
	}
	s.clampCursor()
}

func sameSidebarItem(a, b sidebarItem) bool {
	switch {
	case a.note != nil || b.note != nil:
		return a.note != nil && b.note != nil && a.note.ID == b.note.ID &&
			a.isProjectNote == b.isProjectNote && a.isTemplates == b.isTemplates
	case a.isProjectEntry || b.isProjectEntry:
		return a.isProjectEntry && b.isProjectEntry && a.project != nil &&
			b.project != nil && a.project.Name == b.project.Name
	default:
		return a.isSection == b.isSection && a.isTemplates == b.isTemplates &&
			a.isTasks == b.isTasks && a.state == b.state
	}
}

func (s sidebarModel) update(msg tea.KeyMsg) (sidebarModel, tea.Cmd) {
	items := s.items()

	switch msg.String() {
	case "j", "down":
		if s.cursor < len(items)-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "left":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isSection && item.isTemplates && s.templatesExpanded {
			s.templatesExpanded = false
			newItems := s.items()
			for i, it := range newItems {
				if it.isSection && it.isTemplates {
					s.cursor = i
					break
				}
			}
			s.templatesActive = true
			s.selected = true
			s.selectedNote = nil
		} else if item.isSection && !item.isTemplates && s.expanded[item.state] {
			s.expanded[item.state] = false
			newItems := s.items()
			for i, it := range newItems {
				if it.isSection && it.state == item.state {
					s.cursor = i
					break
				}
			}
			s.activeState = item.state
			s.templatesActive = false
			s.projectsActive = item.state == vault.StateProjects
			s.selected = true
			s.selectedNote = nil
		} else if item.isProjectEntry && s.expandedProjects[item.project.Name] {
			// Collapse project folder.
			s.expandedProjects[item.project.Name] = false
			s.activeProjectName = item.project.Name
			s.projectsActive = true
			s.selected = true
			s.selectedNote = nil
			s.selectedProject = item.project
		} else if item.isProjectNote || (!item.isSection && !item.isTemplates && !item.isProjectEntry) {
			// On a sub-entry: move cursor up to the parent.
			for i := s.cursor - 1; i >= 0; i-- {
				if items[i].isSection || items[i].isProjectEntry {
					s.cursor = i
					break
				}
			}
		}
	case "right":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isProjectEntry && !s.expandedProjects[item.project.Name] {
			// Expand project folder and load its notes.
			s.expandedProjects[item.project.Name] = true
			notes, _ := s.vault.ListAll()
			var pNotes []*vault.Note
			for _, n := range notes {
				if n.Project == item.project.Name && n.State == vault.StateProjects {
					pNotes = append(pNotes, n)
				}
			}
			s.projectNotesByName[item.project.Name] = pNotes
			s.activeProjectName = item.project.Name
			s.projectsActive = true
			s.selected = true
			s.selectedNote = nil
			s.selectedProject = item.project
		} else if item.isSection && item.isTemplates && !s.templatesExpanded {
			s.templatesExpanded = true
			notes, _ := s.vault.ListByTag("template")
			s.templateNotes = notes
			s.templateCount = len(notes)
			s.templatesActive = true
			s.selected = true
			s.selectedNote = nil
		} else if item.isSection && item.isTasks {
			// No expand/collapse state — every activation just selects it.
			s.tasksActive = true
			s.templatesActive = false
			s.selected = true
			s.selectedNote = nil
			s.selectedTasks = true
		} else if item.isSection && !item.isTemplates && !item.isTasks && !s.expanded[item.state] {
			s.expanded[item.state] = true
			if item.state != vault.StateProjects {
				notes, _ := s.vault.ListByState(item.state)
				s.notesByState[item.state] = notes
			}
			s.activeState = item.state
			s.templatesActive = false
			s.projectsActive = item.state == vault.StateProjects
			s.selected = true
			s.selectedNote = nil
		}
	case "enter":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isProjectNote {
			// Note under a project folder: open in main pane.
			s.selectedNote = item.note
			s.selected = true
		} else if item.isProjectEntry {
			// Project folder header: toggle expand/collapse.
			if s.expandedProjects[item.project.Name] {
				s.expandedProjects[item.project.Name] = false
			} else {
				s.expandedProjects[item.project.Name] = true
				notes, _ := s.vault.ListAll()
				var pNotes []*vault.Note
				for _, n := range notes {
					if n.Project == item.project.Name && n.State == vault.StateProjects {
						pNotes = append(pNotes, n)
					}
				}
				s.projectNotesByName[item.project.Name] = pNotes
			}
			// Also open project detail view for this project.
			s.activeProjectName = item.project.Name
			s.projectsActive = true
			s.templatesActive = false
			s.selectedProject = item.project
			s.selected = true
			s.selectedNote = nil
		} else if item.isSection && item.isTemplates {
			if s.templatesExpanded {
				s.templatesExpanded = false
				newItems := s.items()
				for i, it := range newItems {
					if it.isSection && it.isTemplates {
						s.cursor = i
						break
					}
				}
			} else {
				s.templatesExpanded = true
				notes, _ := s.vault.ListByTag("template")
				s.templateNotes = notes
				s.templateCount = len(notes)
			}
			s.templatesActive = true
			s.selected = true
			s.selectedNote = nil
		} else if item.isSection && item.isTasks {
			s.tasksActive = true
			s.templatesActive = false
			s.selected = true
			s.selectedNote = nil
			s.selectedTasks = true
		} else if item.isSection && !item.isTemplates && !item.isTasks {
			if s.expanded[item.state] {
				s.expanded[item.state] = false
				newItems := s.items()
				for i, it := range newItems {
					if it.isSection && it.state == item.state {
						s.cursor = i
						break
					}
				}
			} else {
				s.expanded[item.state] = true
				if item.state != vault.StateProjects {
					notes, _ := s.vault.ListByState(item.state)
					s.notesByState[item.state] = notes
				}
			}
			s.activeState = item.state
			s.templatesActive = false
			s.projectsActive = item.state == vault.StateProjects
			s.selected = true
			s.selectedNote = nil
		} else {
			// Note title selected: open in main pane.
			s.selectedNote = item.note
			s.selected = true
		}
	}
	return s, nil
}

// sidebarContentX is the absolute column (from the terminal's left edge)
// where sidebar row content begins: the outer border (1 char) plus the
// row style's left padding (1 char, see Padding(0, 1) in render()).
const sidebarContentX = 2

// sidebarGlyphHit reports whether x (absolute, terminal-relative) falls on
// an item's "▶"/"▼" expand/collapse indicator rather than its label text.
// Column offsets mirror the literal prefixes built in render(): section rows
// start with a 2-char glyph ("▶ "/"▼ "), project rows with a 2-space indent
// then the 2-char glyph.
func sidebarGlyphHit(item sidebarItem, x int) bool {
	rel := x - sidebarContentX
	switch {
	case item.isTasks:
		return false // no expand/collapse glyph — the whole row just activates
	case item.isSection:
		return rel >= 0 && rel < 2
	case item.isProjectEntry:
		return rel >= 2 && rel < 4
	default:
		return false
	}
}

func (s sidebarModel) render(width, height int, focused bool, openNoteID string) string {
	t := activeTheme
	accentColor := t.BorderNormal
	if focused {
		accentColor = t.Accent
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(accentColor).Padding(0, 1)

	// Cursor: accent bg when focused, dim muted when unfocused.
	cursorFocusStyle := lipgloss.NewStyle().
		Background(t.Accent).Foreground(t.AccentFg).Width(width).Padding(0, 1)
	cursorBlurStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextMuted).Width(width).Padding(0, 1)
	// Active section/project: accent text, no background (cursor is elsewhere).
	accentTextStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.Accent).Width(width).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextPrimary).Width(width).Padding(0, 1)
	noteStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextMuted).Width(width).Padding(0, 1)
	// Open note: accent text, no background.
	noteOpenStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.Accent).Width(width).Padding(0, 1)
	dimNoteStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextDim).Width(width).Padding(0, 1)

	items := s.items()

	var b strings.Builder
	b.WriteString(titleStyle.Render("SECTIONS") + "\n\n")

	// inner = content area width (outer width minus left+right padding).
	inner := width - 2
	if inner < 4 {
		inner = 4
	}

	for i, item := range items {
		isCursor := i == s.cursor
		var line string

		if item.isSection {
			var name string
			var countStr string
			var exp string

			if item.isTemplates {
				exp = "▶ "
				if s.templatesExpanded {
					exp = "▼ "
				}
				name = "#templates"
				countStr = strconv.Itoa(s.templateCount)
			} else if item.isTasks {
				exp = "  " // no expand/collapse glyph — this row has no children
				name = "Tasks"
			} else {
				exp = "▶ "
				if s.expanded[item.state] {
					exp = "▼ "
				}
				name = capitalize(string(item.state))
				countStr = strconv.Itoa(s.counts[item.state])
			}

			// "▶ " is 2 display chars; leave 1 space + countStr at right.
			nameW := inner - 2 - 1 - len(countStr)
			if nameW < 1 {
				nameW = 1
			}
			content := fmt.Sprintf("%s%-*s %s", exp, nameW, name, countStr)

			isActive := (item.isTemplates && s.templatesActive) || (item.isTasks && s.tasksActive) || (!item.isTemplates && !item.isTasks && item.state == s.activeState)
			switch {
			case isCursor && focused:
				line = cursorFocusStyle.Render(content)
			case isCursor:
				line = cursorBlurStyle.Render(content)
			case isActive:
				line = accentTextStyle.Render(content)
			default:
				line = normalStyle.Render(content)
			}
		} else if item.isProjectEntry {
			exp := "  ▶ "
			if s.expandedProjects[item.project.Name] {
				exp = "  ▼ "
			}
			noteCount := len(s.projectNotesByName[item.project.Name])
			badge := ""
			if s.expandedProjects[item.project.Name] && noteCount > 0 {
				badge = " (" + strconv.Itoa(noteCount) + ")"
			}
			titleW := inner - len([]rune(exp)) - len([]rune(badge))
			if titleW < 1 {
				titleW = 1
			}
			content := exp + truncate(item.project.Name, titleW) + badge
			isActive := item.project.Name == s.activeProjectName && s.projectsActive

			switch {
			case isCursor && focused:
				line = cursorFocusStyle.Render(content)
			case isCursor:
				line = cursorBlurStyle.Render(content)
			case isActive:
				line = accentTextStyle.Render(content)
			default:
				line = noteStyle.Render(content)
			}
		} else if item.isProjectNote {
			const indent = "    " // 4 display chars (deeper indent, no dot)
			titleW := inner - len([]rune(indent))
			if titleW < 1 {
				titleW = 1
			}
			content := indent + truncate(item.note.Title, titleW)
			isOpen := openNoteID != "" && item.note != nil && item.note.ID == openNoteID

			switch {
			case isCursor && focused:
				line = cursorFocusStyle.Render(content)
			case isCursor:
				line = dimNoteStyle.Render(content)
			case isOpen:
				line = noteOpenStyle.Render(content)
			default:
				line = noteStyle.Render(content)
			}
		} else {
			const indent = "  · " // 4 display chars
			titleW := inner - len([]rune(indent))
			if titleW < 1 {
				titleW = 1
			}
			content := indent + truncate(item.note.Title, titleW)
			isOpen := openNoteID != "" && item.note != nil && item.note.ID == openNoteID

			switch {
			case isCursor && focused:
				line = cursorFocusStyle.Render(content)
			case isCursor:
				line = dimNoteStyle.Render(content)
			case isOpen:
				line = noteOpenStyle.Render(content)
			default:
				line = noteStyle.Render(content)
			}
		}
		b.WriteString(line + "\n")
	}

	return lipgloss.NewStyle().
		Width(width).Height(height).
		Background(activeTheme.Bg).
		Render(b.String())
}
