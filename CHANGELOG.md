# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com); versioning is
[SemVer](https://semver.org) (pre-1.0: MINOR = new feature/feature-batch,
PATCH = fix-only). See `todo.md` for in-flight work.

## [Unreleased]

### Added
- `:import [path]` command + `I` hotkey — a popover (path field with live
  directory-listing autocomplete, Move/Copy toggle default Move, and a
  destination-state field default Inbox) that imports an external
  markdown file into the vault, renamed to the `<ID> <Title>.md`
  convention with fresh `id`/`created`/`updated`/`state` (existing
  frontmatter `tags:` are preserved). `Vault.Import` in
  `internal/vault/vault.go`; popover in `internal/tui/import_pane.go`.
- `Shift+Tab` now mirrors `Tab` for the global Sidebar ↔ Main pane toggle
  (audited every other Tab site — editor field-cycling and the `:config`
  section switcher already supported Shift+Tab from prior work; palette
  Tab-complete has no natural reverse action and was left alone).
- Persistent "Last saved: HH:MM:SS" footer, fixed at the bottom of the
  document pane, in both the viewer (next to the scroll %) and the editor
  (next to word/line counts). The editor footer also shows an
  "● Unsaved changes" marker whenever the draft's body, tags, project, or
  state differs from the last-saved note.

### Changed
- Removed the transient "saved: `<title>`" message from the global hotkey
  toolbar — save status now lives only in the document-pane footer above.

### Fixed (this batch)
- The editor's and viewer's new footer rows now degrade gracefully
  (dropping the least essential segment, then hard-truncating as a last
  resort) instead of wrapping when the pane is narrow — e.g. a `:split`
  with the editor open in a 3-way layout. A wrapped footer pushed the
  pane's rendered content one line past its bordered box's allotted
  height, desyncing the border. Caught via a second tmux pass in a
  narrow split (the first pass only exercised a single wide pane).

### Fixed
- `Ctrl+Z` undo after `:delete` now re-registers the note's title in the
  in-memory title set, so links pointing at the undone note render as
  working links again instead of staying marked broken.

### Verify
- `go test ./...` — clean.
- `internal/vault/vault_test.go`: `TestImportMovesFileByDefault`,
  `TestImportCopiesFileWhenMoveFalse`,
  `TestImportPreservesExistingTagsButAssignsFreshMetadata`.
- `internal/tui/tui_test.go`: `TestHeadlessImportPopover` (full flow: open
  via `I`, type a path, tab to Mode, toggle to Copy, tab to Destination,
  cycle it, tab to Import, confirm — checks note created, source
  preserved, popover closes, note opened), `TestHeadlessImportPopoverEsc`.
- Manual (tmux): `I` → typed an absolute path to a scratch `.md` file,
  suggestions matched live, tabbed to Mode and toggled to Copy, tabbed to
  Destination and cycled it, tabbed to Import, Enter → popover closed,
  note opened with the source content, source file still present on disk
  (Copy mode confirmed).
- `TestHeadlessNavigation` — `Shift+Tab` toggles Sidebar ↔ Main same as `Tab`.
- `TestEditorFooterShowsDirtyAndSavedTime` — dirty marker appears/disappears
  correctly, timestamp shown matches the note's `Updated` field, saving
  clears the global toast (`m.statusMsg == ""`).
- Manual (tmux): opened a note, `e` to edit, typed a character → footer
  showed "● Unsaved changes"; `Ctrl+S` → marker cleared, timestamp
  advanced, bottom hotkey bar showed no "saved:" text; `Ctrl+Z` reverted
  the test edit afterward.
- `TestFooterRowNeverExceedsWidth` and the width-degradation loop inside
  `TestEditorFooterShowsDirtyAndSavedTime` assert both footers stay ≤
  their pane width at every size down to 0. Manual (tmux): `:split`,
  opened a different note in the second pane, pressed `e` — footer now
  renders on one line (drops the "Tab: cycle fields" hint) instead of
  wrapping and breaking the border, reproducing then confirming the fix
  for the narrow-pane bug above.

Remaining backlog tracked in `todo.md` (import command, soft-delete/trash,
text selection + copy/paste, hotkey-bar grouping).

## [0.1.0] - 2026-07-08

### Added
- Block-style character cursor in view mode: arrow keys move it line-by-line
  and char-by-char; hovering a link/checkbox/code block highlights it; Enter
  activates (open link, toggle checkbox, copy code block via OSC 52).
- Mouse click toggles checkboxes in view mode (same effect as cursor+Enter).
- Mouse wheel scrolling — sidebar, note list, viewer, and help pane, scoped
  to whichever pane is under the cursor, without stealing focus.
- `:delete <note>` command + `D` hotkey — removes the file, undo-safe via
  the existing `Ctrl+Z` undo stack.
- `:search <query>` command — live fuzzy dropdown in the palette (title +
  content matches); arrow to a hit + Enter opens it directly; bare Enter
  opens all hits as a results list; Esc from a note opened this way returns
  to that results list instead of falling through to prior pane history.
- Full H2–H6 markdown heading styles in view mode (previously only H1 was
  visually distinct).
- Edit-mode heading cue: a gutter marker (`▎`) appears next to heading
  lines while typing (chosen over a whole-line accent, which would have
  required forking `bubbles/textarea`).

### Fixed
- Sidebar collapse/expand click only triggers on the row's glyph; clicking
  the label always opens that section/project instead of also toggling it.
- Typing three backticks in the editor no longer auto-closes into four —
  it now expands into a full fenced block with the cursor inside it.
- `Ctrl+Z` undo after `:delete` now re-registers the note's title in the
  in-memory title set, so links pointing at the undone note render as
  working links again instead of staying marked broken.

### Verify
- `go build -o pkm ./cmd/pkm && go vet ./... && go test ./...` — clean.
- `internal/tui/tui_test.go`: `TestHeadlessDeleteThenUndo`,
  `TestHeadlessMouseWheelScrollsViewer`, `TestEditorTripleBacktickOpensFence`,
  `TestHeadlessCheckboxClickToggle`, `TestViewerCursorMovement`,
  `TestHeadlessCursorActivateLink`, `TestHeadlessCursorActivateCheckbox`,
  `TestHeadlessCursorActivateCodeCopy`, `TestHeadlessSearchBareEnterOpensResults`,
  `TestHeadlessSearchNavigatedEnterOpensNote`, `TestHeadlessSearchBackReturnsToResults`,
  `TestHeadlessMouse` (glyph vs. label click).
- Manual (tmux): open a note, arrow keys move a solid block cursor that
  highlights links/checkboxes/code blocks on hover; Enter on each does the
  right thing; `:search doc` → arrow to a hit → Enter opens it → Esc
  returns to the result list, not a new search.
