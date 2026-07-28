package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const helpContent = `NAVIGATION
  ?           toggle this Help view
  j / k       scroll the note viewer down / up
  Tab / Shift+Tab  switch focus: Sidebar ↔ Main pane
  Ctrl+W      cycle to next split pane
  Ctrl+P      open pane picker
  Enter       open note / expand section
  ←           collapse section (sidebar) / glyph toggles a row's expand state
  →           expand section (sidebar)
  [  Alt+←    navigate back in pane history
  ]  Alt+→    navigate forward in pane history
  Esc         go back / close overlay
  Mouse wheel scrolls whichever pane (sidebar, note list, viewer, help) is
  under the cursor, without changing which pane has focus.

VIEW MODE CURSOR
  While reading a note, ↑ ↓ ← → move a solid block cursor character-by-
  character through the rendered text (independent of j/k scrolling, which
  still just scrolls). Moving off-screen scrolls the viewer to follow it.
  When the cursor sits on a link, checkbox, or fenced code block, that
  element is highlighted. Press Enter to act on it:
    - link      → open the linked note (creating it if it doesn't exist)
    - checkbox  → toggle "- [ ]" / "- [x]" and save
    - code block → copy its contents to the clipboard (OSC 52; works over
                    SSH and inside tmux/zellij)
  Space also toggles a checkbox from anywhere on that task's line, not just
  when the cursor sits on the "[ ]" itself; it's a no-op on non-task lines.
  Clicking a link or checkbox with the mouse does the same thing directly.
  Finished tasks sink to the bottom of their block on screen (unfinished
  first, both groups keeping their relative order) and render muted; this
  is display-only, the file's line order never changes. Un-toggling moves
  a task back out of the finished group.
  The viewer's bottom row is a fixed footer: "Last saved: HH:MM:SS" plus
  scroll %. This is separate from the hotkey bar below the pane.

TEXT SELECTION  —  view mode only
  Shift+↑↓←→  extend selection (anchor set on first shift-move)
  Ctrl+A      select the whole note
  Ctrl+C      copy the selection (OSC 52); reports the character count in
              the status bar since a very large copy can be silently
              truncated by the terminal
  Esc         clear the selection (a second Esc then goes back, as usual)
  Any plain (non-shift) cursor move clears the selection.
  Click-drag over body text selects from press to release; releasing
  copies automatically — no Ctrl+C needed for a mouse selection.
  Copied text is always the note's raw Markdown, never the rendered/ANSI
  output or a resolved [[link|alias]]. There is no Ctrl+V here — view mode
  has no editable buffer to paste into.

READING & EDITING
  e           open current note in the editor
  Ctrl+S      save (in editor)
  Ctrl+Space  open the command palette without leaving the editor
  Esc         save (if changed) and close — same as Ctrl+S, just a
              different key; Ctrl+Z undoes it afterward like any save

  While editing, :insert writes straight into the open note — you stay
  in the editor. Any other command (e.g. :archive, :move) saves the
  draft first, then runs normally and exits to the viewer.

  Enter continues "- ", "- [ ] "/"- [x] ", "* ", and numbered lists.
  Pressing Enter again on a marker with no text after it clears that
  marker instead of adding another — the way to break out of a list.

  The editor's footer row shows word/line counts and "Last saved:
  HH:MM:SS"; an "● Unsaved changes" marker appears next to it whenever
  the draft (body, tags, project, or state) differs from the saved note.

  Line operations (body only): Ctrl+L is a leader — the next key runs
  one op, anything else (including Esc) cancels it without typing that
  key into the note.
    Ctrl+L y    yank (copy) the current line
    Ctrl+L d    delete (cut) the current line
    Ctrl+L p    paste the last yanked/deleted line below the cursor
  The register holds one line for this editor session only (cleared on
  close). A deleted line is Ctrl+Z-recoverable like any other save.

IMPORT POPOVER  —  I or :import [path]
  Path field    type or arrow-select a suggestion (live directory listing,
                dirs and .md files, filtered as you type)
  Tab/Shift+Tab move between Path → Mode → Destination → Import
  Space         toggle Move ↔ Copy while the Mode field is focused
                (default: Move — the source file is removed after import)
  ←/→           cycle the destination state while Destination is focused
  Enter         on Path: accept the highlighted suggestion
                on Destination: cycle forward
                on Import: run the import
  Esc           cancel, no changes made
  If Path is a directory, a checklist of its Markdown files appears.
  Move to Files with Tab, select with j/k, and toggle each file with Space;
  only checked files are imported.

EXPORT POPOVER  —  :export [path]
  Writes the open note's raw markdown (frontmatter included) to a path
  outside the vault — always a copy, the vault note is never touched.
  Path field    prefilled with the note's own filename; type or
                arrow-select a suggestion (same directory-listing
                autocomplete as Import)
  Tab/Shift+Tab move between Path → Export
  Enter         on Path: accept the highlighted suggestion
                on Export: run the export (an existing target asks for
                one more Enter to confirm the overwrite)
  Esc           cancel, no changes made

TASK OVERVIEW  —  :tasks or the sidebar's Tasks row
  Scans the whole vault for checkbox lines, grouped: each active project
  with task-bearing notes (heading), its notes (sub-heading, sorted by
  title) then their tasks; a trailing "Unassigned" group covers every
  other task-bearing note the same way. Read-only in v1 — Enter opens
  the source note, toggling from here isn't supported yet.
  j/k or ↓/↑    move the row cursor
  g / G         jump to first / last row
  Enter         open the source note of the task under the cursor
  r             refresh configured GitLab issue sections
  Esc           close, back to the note list
  GitLab issues are read-only virtual rows configured under :config →
  Issues. Set PKM_GITLAB_TOKEN (PAT with read_api); Enter opens a cached
  issue detail and fetches comments live.

TRASH  —  :trash
  :delete moves a note here instead of removing it outright — recoverable
  for a configurable retention window (default 30 days, :config → General).
  Purged automatically on a future startup once past that window.
  j/k or ↓/↑    move the row cursor
  g / G         jump to first / last row
  Enter         restore the note (falls back to Inbox if its project is
                gone)
  d             permanently delete — press again to confirm (footer line,
                not a popup); any other key cancels. Irreversible.
  Esc           close, back to the note list

COMMAND PALETTE  —  press : or Ctrl+Space to open
  :new "Title"              create note in Inbox
  :new project <name>       create a new project (does not assign the open note)
  :add project <name>       assign open note to a project (moves to Projects,
                             creates the project if it doesn't exist yet)
  :new template "Title"     create a new template note
  :insert <name>            insert a template into the current note
  :insert var <name>        insert a variable's value at the cursor (edit mode)
  :open <query>             open by title; or full-text search; or #tag filter
  :search <query>           fuzzy search titles and content
                            ↑↓ to a hit + Enter opens it directly; a bare
                            Enter opens every hit as a list (Esc from a note
                            opened this way returns to that list)
  :move <note> → <state>    move note to a state
  :archive <note>           archive a note
  :delete <note>            move a note to trash (Ctrl+Z undoes it
                            immediately; :trash recovers it later)
  :import [path]            open the import popover (path autocomplete,
                            move/copy toggle, destination state)
  :export [path]            open the export popover (path autocomplete,
                            prefilled with the open note's filename)
  :tasks                    show every task in the vault, grouped by
                            project then file (also in the sidebar)
  :trash                    list deleted notes; Enter restores, d
                            permanently deletes
  :reindex                  rescan notes, normalize manually added .md
                            files into Inbox, and atomically rebuild search
  :split [note]             open a new side-by-side pane
  :close                    close the focused pane
  :config                   open settings (General / Keybindings / Variables / Issues)
  :config export [path]     write config to file (default .pkm/config-export.yaml)
  :config import [path]     load config from file
  :quit  /  :exit           quit (session is saved)
  :help                     show this page

  Autosuggest: ↑/↓ to navigate, Tab to complete, Enter to run, Esc to cancel.

QUIT
  :quit  :exit              save session and quit
  Ctrl+Q                    save session and quit
  Ctrl+D                    save session and quit (Unix EOF convention)

SHIFT SHORTCUTS  (open palette pre-filled, except I which opens directly)
  N  :new       O/S  :open      A  :archive
  M  :move      T    :insert    P  :add project
  D  :delete    I    :import (opens the popover directly, no palette)

CTRL SHORTCUTS
  Ctrl+C  cancel / close    same as Esc in every mode
  Ctrl+P  panes             open pane picker
  Ctrl+Q  quit              save session and quit
  Ctrl+W  next pane         cycle split pane focus
  Ctrl+Z  undo              undo last note save
  Ctrl+Y  redo              redo

PROJECT WORKFLOW
  1. Open a note (it starts in Inbox).
  2. Press P or type :add project <name> to assign it to a project.
     → The note moves to Projects automatically.
     → If the project name is new, it is created. Max 4 active projects.
  3. In the sidebar, expand Projects → expand the project folder to see its notes.
  4. Click the project folder header to open the project detail page:
     - See all attached notes.
     - Read the history log (attach / detach events).
     - Press e to add a Hemingway bridge entry (timestamped journal note).
  5. To move a note out of a project: :move <note> → inbox  (or any other state).
  6. Projects section shows an overview when no specific project is selected.

TEMPLATE WORKFLOW
  Creation templates  (applied automatically when creating a note):
    Place a .md file in vault/templates/.
    Variables: {{id}}  {{title}}  {{created}}  {{updated}}

  Insert templates  (appended to the open note on demand):
    Tag any note with "template" in its frontmatter (or use :new template "Title").
    Press T or type :insert <name> to insert it into the current note.
    Insert templates appear in #templates in the sidebar.
    {{id}} {{title}} {{created}} {{updated}} and any :config-defined
    variable are substituted on insertion.

CONFIGURATION  —  :config  (Tab cycles General / Keybindings / Variables / Issues)
  Keybindings section: remaps the global chords (palette, pane picker, next
  pane, quit, undo, redo, save) that terminal multiplexers like tmux/zellij
  may otherwise intercept. Enter captures the next ctrl/alt keypress; d
  resets to default.
  Variables section: simple key-value pairs used by :insert var <name> and
  by {{name}} substitution in :insert <template>. Enter adds/edits a value,
  d deletes.
  Issues section: GitLab base URL plus project paths (group/repo). Set the
  read_api PAT in PKM_GITLAB_TOKEN; it is never written to config or cache.
  :config export / :config import move a whole config.yaml between vaults;
  importing an older or newer file never crashes — unknown fields are
  ignored and missing ones fall back to defaults.

INDEXING
  The vault is reconciled in the background at startup and whenever an
  unformatted Markdown file is added to notes/. Progress appears in the
  bottom status bar. :reindex runs the same scan manually. A failed or
  interrupted scan leaves the previous SQLite index intact; restart pkm or
  run :reindex to retry. Files already normalized remain valid and are not
  duplicated.

LINKS
  [[Note Title]]              link to another note
  [[Note Title|Display Text]] link with alias (renders as "Display Text")
  Click a link in the viewer to open it. Missing notes are created on open.

KNOWLEDGE MODEL  (PARA-inspired)
  Inbox → Projects / Areas / Research → Archive
  All new notes land in Inbox.
  Use :move to promote notes between states.
  Max 4 active projects at a time.
`

// helpLines renders all help content lines styled and clamped to width.
// Each element in the returned slice is one display row.
func helpLines(width int) []string {
	t := activeTheme

	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	normal := lipgloss.NewStyle().Foreground(t.TextPrimary)
	muted := lipgloss.NewStyle().Foreground(t.TextSecond)
	dim := lipgloss.NewStyle().Foreground(t.TextDim)
	blank := lipgloss.NewStyle().Background(t.Bg)

	clamp := func(s string) string {
		if utf8.RuneCountInString(s) > width {
			runes := []rune(s)
			s = string(runes[:width])
		}
		return s
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(helpContent, "\n"), "\n") {
		var rendered string
		switch {
		case line == "":
			rendered = blank.Render(strings.Repeat(" ", width))
		case line[0] != ' ':
			// Section heading — occupies full width
			rendered = heading.Width(width).Render(clamp(line))
		default:
			trimmed := strings.TrimLeft(line, " ")
			indent := len(line) - len(trimmed)
			// Try to split into key/desc on a double-space gap within the first 28 chars.
			idx := strings.Index(trimmed, "  ")
			if idx > 0 && idx < 28 &&
				!strings.HasPrefix(trimmed, "→") &&
				!strings.HasPrefix(trimmed, "-") &&
				!strings.HasPrefix(trimmed, "Place") &&
				!strings.HasPrefix(trimmed, "Tag") &&
				!strings.HasPrefix(trimmed, "Press") &&
				!strings.HasPrefix(trimmed, "Variables") {
				key := trimmed[:idx]
				desc := strings.TrimSpace(trimmed[idx:])
				prefix := strings.Repeat(" ", indent)
				raw := prefix + key + "  " + desc
				if utf8.RuneCountInString(raw) > width {
					desc = clamp(desc)
				}
				rendered = strings.Repeat(" ", indent) + muted.Render(key) + "  " + normal.Render(desc)
			} else {
				rendered = dim.Render(clamp(line))
			}
		}
		// Styled key/description rows are assembled from multiple fragments,
		// so clamping the individual description above is not enough to
		// account for the key and indentation. Lip Gloss wraps an over-wide
		// row when the pane box is rendered, making the whole application
		// taller than the terminal. Keep every logical help row to one
		// physical display row before windowing it.
		if lipgloss.Width(rendered) > width {
			rendered = xansi.Truncate(rendered, width, "")
		}
		lines = append(lines, rendered)
	}
	return lines
}

// HelpTotalLines returns the total number of content lines for scroll clamping.
func HelpTotalLines() int {
	return len(strings.Split(strings.TrimRight(helpContent, "\n"), "\n"))
}

// renderHelpView renders the help as a scrollable document windowed to [scrollOff, scrollOff+height).
func renderHelpView(width, height, scrollOff int) string {
	t := activeTheme
	lines := helpLines(width)

	// Clamp scroll offset.
	maxOff := len(lines) - height
	if maxOff < 0 {
		maxOff = 0
	}
	if scrollOff > maxOff {
		scrollOff = maxOff
	}
	if scrollOff < 0 {
		scrollOff = 0
	}

	// Window to visible slice.
	end := scrollOff + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scrollOff:end]

	// Pad with blank lines to fill the pane exactly.
	blank := lipgloss.NewStyle().Width(width).Background(t.Bg).Render("")
	for len(visible) < height {
		visible = append(visible, blank)
	}

	return strings.Join(visible, "\n")
}
