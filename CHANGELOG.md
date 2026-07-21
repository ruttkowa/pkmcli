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
- `:tasks` command + sidebar "Tasks" row (below `#templates`, shown by
  default) — a read-only Task Overview collecting every checkbox line in
  the vault, grouped by active project (sidebar order) then by file
  (title order), with a trailing "Unassigned" group for everything else.
  `j`/`k`/`g`/`G` move the row cursor, `Enter` opens the task's source
  note, `Esc` closes. Assembled fresh from a vault scan each time it's
  opened — no persistent task index yet. Issue #13; foundation for the
  rest of the task epic. `internal/tui/task_overview.go`.
- `:config` → General gets two new toggles: "Show Tasks nav" and "Show
  Templates nav" (both default on), independently controlling whether
  the sidebar's Tasks row and `#templates` section appear at all.
  Applied live on toggle, persisted to `config.yaml`
  (`show_tasks_nav`/`show_templates_nav`), and safe for configs written
  before these fields existed — a config seeds `defaultConfig()` (both
  true) before unmarshalling, so an absent key leaves the seeded true in
  place while an explicit `false` is still honored. Issue #14.
  `internal/tui/appconfig.go`, `config_pane.go`, `sidebar.go`.
- Finished tasks now sink to the bottom of their block in the viewer
  (unfinished first, both groups keeping original relative order) and
  render in a muted, secondary style — display-only, per user decision:
  the `.md` file's line order is never rewritten, only the rendered
  position. The checkbox stays toggleable in its new slot either way.
  User-approved decision (render-only, not a physical file rewrite).
  Raw-line integrity across the reorder verified by a discriminating
  test with duplicate task text. Issue #12. `internal/tui/viewer.go`
  (`annotateInteractive`'s per-block stable partition,
  `processCheckboxesAndCode`'s muted re-tint).

### Changed
- Removed the transient "saved: `<title>`" message from the global hotkey
  toolbar — save status now lives only in the document-pane footer above.
- Task checkbox lines may now optionally carry a completion date and a
  result: `- [x] Task ✅ 2026-07-10 --> result`, where a result that parses
  as `[[wikilink]]` renders/opens as a link. Stamped/stripped automatically
  by `toggleCheckboxLine` on toggle; plain `- [x] text` with neither stays
  fully valid and untouched until actually toggled. Explicitly approved
  departure from CLAUDE.md's prior "no custom task syntax" — documented in
  both `CLAUDE.md` and `spec.html`. Foundation for the task-overview epic
  (see `todo.md`/GitHub issues #11-#14). `internal/tui/viewer.go`
  (`parseTaskLine`, `formatTaskLine`, `toggleCheckboxLine`).

### Fixed (this batch)
- The editor's and viewer's new footer rows now degrade gracefully
  (dropping the least essential segment, then hard-truncating as a last
  resort) instead of wrapping when the pane is narrow — e.g. a `:split`
  with the editor open in a 3-way layout. A wrapped footer pushed the
  pane's rendered content one line past its bordered box's allotted
  height, desyncing the border. Caught via a second tmux pass in a
  narrow split (the first pass only exercised a single wide pane).

### Fixed (bug-fix pass)
- Vault: fixed a leading-blank-line accretion bug where every save→reload
  round trip (which every checkbox toggle triggers, via the fsnotify
  watcher) permanently added one more blank line to the top of a note's
  body — unbounded growth on repeated use of the task feature. Root cause:
  `parseNote` only stripped one leading `\n` after the frontmatter
  delimiter, while `marshalNote` always re-added one, so any blank line
  already baked into `Body` survived and compounded. Fixed by stripping
  all leading newlines on parse (`strings.TrimLeft` instead of
  `TrimPrefix`) so the round trip is idempotent; this also self-heals
  notes already corrupted by the bug the next time they're loaded, no
  migration needed. Found while manually verifying #12; confirmed via a
  discriminating test (temporarily reverted the fix to prove it fails)
  plus a live tmux run toggling a real checkbox four times. Discovered,
  diagnosed, and fixed within this session — never released.
  `internal/vault/frontmatter.go` (`parseNote`).
- Editor: `Esc` now saves the draft (if changed) and pushes an undo
  record, instead of silently discarding every typed change. Routes
  through the same `commitEditorDraft` path as `Ctrl+S`; a clean
  (unmodified) `Esc` is still a no-op save/undo push.
  `internal/tui/model.go`.
- Viewer: toggling a checkbox no longer snaps the cursor and scroll
  position back to the top. The fsnotify-triggered reload
  (`vaultChangedMsg` → `withNote`) was resetting them; now saved and
  restored around the reload, same pattern `applyCheckboxToggle` already
  used. `internal/tui/model.go`.
- Viewer: `Space` toggles the checkbox on the current line from anywhere
  in that line, not just when the cursor sits on the `[ ]` itself — a
  no-op on non-task lines. `internal/tui/viewer.go`.
- Editor: pressing `Enter` again on a list/task marker with no text
  after it (i.e. right after Enter auto-continued the list) now clears
  that marker instead of adding yet another one, letting you break out
  of a list. Terminal-safe stand-in for the originally requested
  `Shift+Enter`: this Bubble Tea/terminal stack delivers Shift+Enter as
  an identical `KeyMsg` to plain Enter (verified empirically), so there
  is no separate key event to bind. `internal/tui/editor.go`
  (`handleEnter`).
- Viewer: fenced ` ``` ` code blocks now render with the same flat
  color/background swatch as inline `` `code` `` spans, instead of
  glamour's default per-token chroma syntax highlighting (visibly
  different from inline code). `internal/tui/viewer.go`
  (`headingStyleConfig`, which disables `CodeBlock.Chroma` and copies
  the `Code` swatch onto `CodeBlock`).

### Fixed
- `Ctrl+Z` undo after `:delete` now re-registers the note's title in the
  in-memory title set, so links pointing at the undone note render as
  working links again instead of staying marked broken.

### Fixed (issue batch, 2026-07-21)
- Editor: assigning a project via the Project field now does everything
  `:add project`/Shift+P already did — creates the project if new
  (respecting the max-4-active-projects limit), forces the note into
  `projects` state, records attach/detach history, and reveals the note
  in the sidebar's expanded project folder. Previously `commitEditorDraft`
  only wrote the `Project` string, so an edited note pointed at a project
  it never actually joined and stayed invisible in the nav tree. Clearing
  the field now records a detach and returns the note to Inbox instead of
  leaving it orphaned in `projects` state with no project. A max-projects
  error now leaves the draft and on-disk note untouched with the editor
  still open, instead of silently discarding the edit. Issue #25.
  `internal/tui/model.go` (`commitEditorDraft`), `internal/tui/commands.go`
  (`assignProjectToNote`, factored out of `cmdProject` and shared by both
  paths).
- Editor: the Project field now autosuggests from active projects
  (case-insensitive prefix match, same convention as the palette's
  `:add project` argument), rendered as a dropdown under the field;
  `Tab`/`Enter` accepts, `Esc` dismisses the list without leaving the
  field. Issue #25. `internal/tui/editor.go`.
- Viewer: a finished (checked) task now dims fully when glamour word-wraps
  it onto multiple rendered lines, instead of only its first line — the
  `cbMark` sentinel used to be the sole dimming trigger and only ever
  lands on line one. The dim run now continues across continuation lines
  and stops at the next blank line, the next task, or a code-fence
  boundary (never by indentation, which glamour also uses for nested list
  items). Issue #17. `internal/tui/viewer.go`
  (`processCheckboxesAndCode`).
- Editor: `Backspace` on an auto-paired `()`, `[]`, or `` `` `` with the
  cursor directly between the two (nothing typed in between) now deletes
  both halves instead of leaving the closer orphaned — previously
  deleting `(` from `(|)` left `)` behind, and the next `(` produced
  `())`. `Backspace` with any content between the pair still deletes a
  single character as before. Issue #21. `internal/tui/editor.go`
  (`updateBody`'s new `"backspace"` case, `charBeforeCursor`).
- Fixed the actual root cause of #18 (reported as sidebar mouse clicks
  landing "about 1.5 lines" off target): the bottom hotkey/tooltip bar
  had no width safety net, so at terminal widths too narrow for its full
  chip list (confirmed empirically: any width under ~150 columns with
  the default global bar), `lipgloss.Style.Width` word-wrapped it onto a
  second line instead of clipping — silently rendering one row taller
  than the requested window height (measured: a 100x40 window rendered
  41 lines). A real terminal has to scroll to show that extra row, which
  desyncs every mouse click's Y coordinate from the row the app's layout
  math assumes — `handleMouseClick`'s `y - 4` offset itself was already
  correct, exactly as the original #18 triage found. Added
  `fitTooltipBar`, the same hard-truncate safety net the editor/viewer
  footers already use, applied to every exit path of
  `renderTooltipBar`. `internal/tui/model.go`.
  **Separately discovered, left unfixed (out of scope for #18):** the
  help view (`?`) overflows far worse at narrow widths (measured up to
  11 rows over budget at width 30) — a different bug, its content pane
  isn't clipped to the layout's content height at all. Not a mouse-click
  issue and not touched here.

### Added (issue batch, 2026-07-21)
- `install.sh` — builds `pkm` and installs it to a directory on `PATH`
  (default `~/.local/bin`, overridable via `PREFIX=` or a positional
  arg). If the install directory isn't already on `$PATH`, appends an
  `export PATH=...` line to the shell rc file chosen from `$SHELL`
  (`.zshrc`/`.bashrc`); idempotent (checks before appending), never
  rewrites existing rc content, and prints the line instead of guessing
  for any other shell. Aborts with a non-zero exit and installs nothing
  if the build fails. Issue #16.

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

- `TestHeadlessEditorAssignsProject`, `TestHeadlessEditorClearsProject`,
  `TestHeadlessEditorProjectAssignmentAtMaxLeavesDraftIntact`,
  `TestEditorProjectSuggestPrefixMatch` (#25).
- Manual (tmux): opened a note, Shift+Tab into the Project field, typed a
  prefix — dropdown appeared under the field; `Tab` accepted "Homelab";
  `Ctrl+S` → note showed `State: projects · Project: Homelab`, sidebar
  auto-expanded Projects → Homelab with the note visible inside.
- `TestProcessCheckboxesAndCodeDimsWrappedContinuationLines`,
  `TestProcessCheckboxesAndCodeUnfinishedWrappedTaskUnchanged`,
  `TestProcessCheckboxesAndCodeDimStopsAtNextTask` (#17) — unit-level on
  `processCheckboxesAndCode` with synthetic ANSI input, same pattern as
  the existing `TestProcessCheckboxesAndCodeMutesFinishedTasks`, since
  lipgloss emits no color codes without a real terminal color profile
  (confirmed empirically: a first pass through the full viewer+glamour
  pipeline in a headless test produced zero ANSI color codes).
- Manual (tmux): a long finished task wrapped across 3 rendered lines —
  captured with `tmux capture-pane -e` to inspect the raw escape
  sequences, all 3 lines carried the identical dim color code
  (`38;2;76;86;105`), while a following unfinished task kept a different,
  undimmed color — confirming the fix live, not just at the unit level.
- `TestHeadlessBackspaceDeletesEmptyAutoPair`,
  `TestHeadlessBackspaceOnNonEmptyPairDeletesOnlyContent`,
  `TestHeadlessBackspaceThenReopenNoDoubleClose`,
  `TestHeadlessBackspaceClosingBracketPairDismissesLinkSuggest` (#21),
  covering all three pair types plus the reported regression and the
  link-autosuggest interaction.
- Manual (tmux): typed `(` → `()`; `Backspace` → empty; typed `(` again →
  `()`, not `())` — reproduced then confirmed the fix for the exact
  reported regression.
- `TestHeadlessViewNeverExceedsRequestedHeight` (#18) — asserts the
  rendered frame is exactly the requested height across widths 40-200,
  in the default and config-overlay modes (a scratch measurement caught
  the actual bug first: a 100x40 window rendered 41 lines before the
  fix, 40 after, across every width from 40 to 200).
  `TestFitTooltipBarNeverWrapsToASecondLine` covers `fitTooltipBar`
  directly.
- Manual (tmux): at 100x40 — the size that reproduced the bug — the
  breadcrumb row (previously invisible, scrolled off by the overflow)
  is back; sent a raw SGR mouse-click escape sequence at the exact
  row/column the sidebar layout math expects for the Inbox glyph, and it
  correctly toggled Inbox's expand state with no offset.
- Manual, `install.sh` (#16): fresh install with the target dir off
  `$PATH` (binary lands, correct rc file gets exactly one export line,
  both for `zsh` and `bash`); run twice → rc file byte-identical; target
  dir already on `$PATH` → no rc modification; positional custom install
  dir → binary lands there; unrecognized `$SHELL` → prints the line,
  touches nothing; build broken (`go` absent from `$PATH`) → non-zero
  exit, nothing installed, no rc file; `git status` stays clean after a
  build (binary ignored, `install.sh` itself is the only new file).

Remaining backlog tracked as GitHub issues (repo `ruttkowa/pkmcli`):
soft-delete/trash (#1), text selection + copy/paste (#2), hotkey-bar
grouping (#3), line-wise editor operations (#10).

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
