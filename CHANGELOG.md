# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com); versioning is
[SemVer](https://semver.org) (pre-1.0: MINOR = new feature/feature-batch,
PATCH = fix-only). See `todo.md` for in-flight work.

## [Unreleased]

### Fixed
- Help content and the breadcrumb are hard-clipped to their single-row
  width budgets, preventing the Help view from growing beyond the terminal
  frame at narrow widths. Issue #26.

## [0.2.0] - 2026-07-22

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
- The cursor no longer jumps to the top/bottom of the note when switching
  between view and edit mode. Two independent bugs, both fixed:
  - **Edit → View (the reported "jump on save"):** `commitEditorDraft`
    called `viewer.withNote` (which resets `cursorRow` to 0) without ever
    restoring a position afterward, unlike the checkbox-toggle path
    (`applyCheckboxToggle`), which always has. Now reads the textarea's
    cursor line before committing and maps it to the freshly re-rendered
    body's matching line.
  - **View → Edit (found while fixing the above — a second, independent
    root cause of the same "jump" complaint):** `newEditPane` called
    `ta.Update(KeyCtrlHome)` *before* `ta.Focus()`, but
    `bubbles/textarea.Update` is a no-op while unfocused — so the
    intended "reset to the top" never actually ran, and the editor always
    opened with the cursor wherever `SetValue` had left it (the
    document's end). Reordering to focus-then-reset fixed the no-op, and
    the editor now also starts at the raw line the viewer's cursor was
    on, not the top.

  Both directions needed a general rendered-line → raw-line map, which
  didn't exist before (`checkboxLines`/`headingLines` only cover their
  own element types). Line-level and best-effort by design, not
  character-exact — glamour reflow means most rendered lines have no
  exact raw counterpart (word-wrap, list indentation, link aliasing all
  change line count/content), so a per-source-line sentinel would either
  collide with itself across a wrapped paragraph or require placement
  before syntax-critical leading characters (breaking list/blockquote/hr
  detection). Resolved by marking only the *first* raw line of each
  paragraph-like block (reusing the existing PUA-sentinel technique,
  placed only on lines proven safe to prefix — unindented, no leading
  `#`/`-`/`*`/`+`/`>`/`|`/`` ` ``/`~`/ordered-list-marker), and
  forward-filling every rendered line in between from the nearest
  preceding anchor — the same fallback the qualification spec authorized
  for lines with no exact rendered counterpart. Issue #22.
  `internal/tui/viewer.go` (`lineMark`, `lineRef`, `ordinaryLineSafe`,
  `rawLines` field, `rawLineAt`, `renderedLineForRaw`), `internal/tui/model.go`
  (`commitEditorDraft`), `internal/tui/editor.go` (`newEditPane`).

### Added (issue batch, 2026-07-21)
- `install.sh` — builds `pkm` and installs it to a directory on `PATH`
  (default `~/.local/bin`, overridable via `PREFIX=` or a positional
  arg). If the install directory isn't already on `$PATH`, appends an
  `export PATH=...` line to the shell rc file chosen from `$SHELL`
  (`.zshrc`/`.bashrc`); idempotent (checks before appending), never
  rewrites existing rc content, and prints the line instead of guessing
  for any other shell. Aborts with a non-zero exit and installs nothing
  if the build fails. Issue #16.
- `:export [path]` command + popover, modeled on `:import` (reuses
  `pathSuggestions`' directory-listing autocomplete): writes the open
  note's raw markdown — frontmatter included, byte-identical to the
  vault file, so it round-trips back in via `:import` — to a path
  outside the vault. Always a copy; the vault note is never modified,
  moved, or reindexed. Path field prefills with the note's own
  `<ID> <Title>.md` filename. A bare directory path exports under that
  filename; a nonexistent target directory errors instead of being
  created implicitly; an existing target requires one extra `Enter` to
  confirm the overwrite; no note open shows an error instead of an empty
  prompt. Issue #23. `internal/tui/export_pane.go`,
  `internal/tui/commands.go` (`cmdExport`, `runExport`).
- Note viewer: headings are now foldable. Every heading shows a `▼`/`▶`
  glyph; `←` on a heading (block cursor) collapses it, `→` expands it,
  and clicking a heading toggles it. Collapsing hides everything until
  the next heading of the same or higher level, including nested
  sub-headings' own lines regardless of their individual fold state.
  Folded before rendering (dropped raw lines never reach glamour), so
  the existing rendered-line→raw-line maps (links, checkboxes) stay
  correct for content after a fold — verified against the exact
  index-shift regression a naive after-render approach would hit.
  View-only, per-note, never written to the note; explicitly preserved
  (not reset) across a checkbox-toggle reload of the same note, the same
  way cursor/scroll position already was. Issue #20. `internal/tui/viewer.go`
  (`hiddenLinesForFold`, `annotateInteractive`, `processCheckboxesAndCode`),
  `internal/tui/model.go` (`applyFold`, `toggleFoldAt`).
- Project Detail: the previously-static pane now has a fully interactive
  **Tasks** section — a per-project filtered view of the Task Overview,
  grouped by note. `j`/`k` move the cursor (skipping note-header rows,
  clamped at both ends), `Space` toggles the checkbox under the cursor
  through the same `toggleCheckboxLine` path the note viewer uses (✅-date
  stamping included), `Enter` jumps to the task's source note. An empty
  project shows a plain "(no tasks)" line. The pane's two focus states
  (task list vs. the pre-existing Hemingway-bridge text input) are the
  existing `editingBridge` flag reused, entered/left the same way as
  before (`e`/`i`, `Esc`, or a submitted `Enter`) — a `Tab`-based focus
  switch was considered but dropped: it collides with the global
  Sidebar↔Main pane toggle once the bridge isn't already focused, so it
  couldn't reliably enter bridge focus. Issue #19. `internal/tui/project_views.go`
  (`projectDetailPane`'s `taskRows`/`taskCursorRow`/`moveTaskCursor`),
  `internal/tui/task_overview.go` (`projectTaskRows`, factored out of
  `buildTaskOverviewRows` and shared by both), `internal/tui/model.go`
  (`toggleCheckboxOnNote`, factored out of `applyCheckboxToggle` and
  shared with the new `toggleProjectDetailTask`).
- Editor: line-wise yank/delete/paste, body only. `Ctrl+L` is a leader —
  the next key runs one op (`y` yank, `d` delete/cut, `p` paste the
  register below the cursor line); anything else, including `Esc`,
  cancels the chord and discards that key instead of typing it into the
  note. Direct `Ctrl+Y`/`Ctrl+D`/`Ctrl+P` bindings (the original
  suggestion) turned out to already be taken — Redo, Quit-alias, and
  Pane-Picker/cursor-up respectively — so the user picked the leader
  chord instead, the only free key. The register lives for the editor
  session only (reset on close, no cross-note carry). Mutates the same
  `bubbles/textarea` value the normal save path commits, so a deleted
  line is `Ctrl+Z`-recoverable like any other edit — no separate undo
  plumbing needed. The chord interception had to move up to `editPane`'s
  outer key switch (mirroring the existing link/project-suggestion
  blocks), not inside `updateBody` where it was first written: the outer
  switch's own unconditional `"esc"` case (cancels the whole editor)
  would otherwise steal Escape before a pending chord ever saw it,
  turning "Ctrl+L then Esc" (abort just the chord) into closing the
  editor. Issue #10. `internal/tui/editor.go` (`lineOpPending`,
  `lineRegister`, `yankCurrentLine`, `deleteCurrentLine`,
  `pasteLineBelow`, `setCursorLine`).

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
- `internal/tui/tui_test.go` (#23): `TestHeadlessExportPrefillsCurrentNoteFilename`,
  `TestHeadlessExportWritesByteIdenticalCopy`,
  `TestHeadlessExportExistingTargetRequiresConfirm`,
  `TestHeadlessExportNonexistentDirectoryErrors`,
  `TestHeadlessExportDirectoryPathExportsUnderNoteFilename`,
  `TestHeadlessExportNoNoteOpenShowsError`.
- Manual (tmux): created a note, `:export`, popover showed the prefilled
  filename; cleared it, typed an out-of-vault target path, confirmed —
  popover closed, exported file byte-identical to the vault copy, vault
  copy unchanged.
- `internal/tui/tui_test.go` (#20): `TestHiddenLinesForFoldHidesNestedHeadingRegardlessOfOwnState`,
  `TestHeadlessFoldLinkAndCheckboxBelowFoldStillResolve` (the index-shift
  regression), `TestHeadlessFoldCollapseExpandRoundTrips`,
  `TestHeadlessFoldKeyboardLeftRightOnHeading`,
  `TestHeadlessLeftRightOnNonHeadingStillMovesCursor`,
  `TestHeadlessFoldCollapseNearEndNoPanic`,
  `TestHeadlessMouseClickOnHeadingTogglesFold`.
- Manual (tmux): typed a note with two `##` headings, a task line, and a
  self-link in the second section; both headings showed `▼`; clicking
  the first heading collapsed it to `▶` and hid its content, leaving the
  second heading and its content (now shifted up) untouched; clicked the
  self-link past the fold — it opened correctly (confirmed by the fold
  resetting on the fresh note-open), proving the link resolved to the
  right target despite the shifted rendered position. Clicked the first
  heading again to re-expand it.
- `internal/tui/tui_test.go` (#19): `TestProjectDetailTaskRowsGroupedByNoteFiltered`,
  `TestProjectDetailEmptyTasksNoPanic`,
  `TestProjectDetailCursorSkipsHeadersAndClamps`,
  `TestHeadlessProjectDetailToggleTaskStampsDate`,
  `TestHeadlessProjectDetailEnterJumpsToTaskSourceNote`.
- Manual (tmux): opened a project with two task-bearing notes; cursor
  started on the first task, `j` moved onto the second task, then across
  the second note's header directly onto its first task (header row never
  selected, confirmed via `capture-pane -e`); `Space` toggled a task —
  rendered `✅` date appeared immediately and the on-disk file showed the
  same stamped line; typed into the bridge (`e`, then `j`/`j`) — task
  cursor didn't move, `Esc` returned to the task list unaffected; `Enter`
  on a task opened its source note in the main pane. A project with no
  tasks rendered "(no tasks)" with no panic and `j`/`k`/`Space`/`Enter`
  all safe no-ops.
- `internal/tui/tui_test.go` (#22): `TestViewerRawLineMapNotIdentityAfterWrap`
  (the map's core requirement — a rendered line after a wrapped
  paragraph, and a checkbox line further down, both resolve to their
  correct, non-identity raw lines), `TestHeadlessViewEditViewRoundTripPreservesRawLine`
  (view→edit→view on a wrapped note lands the textarea cursor on the
  right raw line, and returns the viewer cursor to it after save),
  `TestHeadlessEditorSaveDoesNotJumpViewerCursorToTop` (the minimal
  regression case for the literal reported complaint).
- Manual (tmux): a 40-section, ~240-line note, each section a wrapping
  paragraph + a checkbox; scrolled deep in with the block cursor (`↓` —
  not `j`, which only scrolls), pressed `e` — the editor's viewport
  opened already scrolled to the matching raw lines, not the top;
  `Ctrl+S` with no changes — viewer returned to the same scroll position
  (8%), not the top; repeated with an actual edit (typed a word at the
  cursor position, confirmed it landed on the intended line) — save
  still returned to the same position, not the top. Also caught, while
  debugging this same manual pass, the pre-existing `ta.Update`-before-
  `Focus` no-op described above — confirmed via a scratch test showing
  `Ctrl+Home` had no effect until reordered.
- `internal/tui/tui_test.go` (#10): `TestHeadlessLineOpsYankAndPaste`,
  `TestHeadlessLineOpsDeleteThenUndo`,
  `TestHeadlessLineOpsDeleteLastLineClampsCursor`,
  `TestHeadlessLineOpsPasteWithEmptyRegisterIsNoop`,
  `TestHeadlessLineOpsAbortOnUnrecognizedKeyDiscardsIt` (covers the
  Esc-aborts-chord-not-editor case specifically),
  `TestHeadlessLineOpsChordOnlyInBody`.
- Manual (tmux, including the explicit "does a terminal swallow Ctrl+L"
  check the issue called out): typed three lines, `Ctrl+L y` on the
  last — no character inserted, confirming the key was consumed, not
  typed; `Ctrl+L p` duplicated it; `Ctrl+L d` on a middle line removed
  it; `Ctrl+S` — the saved file on disk matched exactly; `Ctrl+Z` reverted
  the whole session's edits. Confirmed `Ctrl+L` while a header field
  (Project) was focused did nothing — the key fell through to that
  field's own input instead.

### Added (issue batch, 2026-07-22)
- **Trash / soft-delete with configurable retention.** `:delete` (and `D`)
  now move a note into `<vault>/.pkm/trash/` instead of permanently
  removing it, recorded in a `.pkm/trash.json` sidecar (`deleted_at`,
  original path, and the state/project it had) — the note's own
  frontmatter is untouched, per the locked decision to keep `spec.html`'s
  schema unchanged. Recoverable via the new `:trash` command (a list view
  modeled on the Task Overview: `j`/`k`/`g`/`G` to navigate, `Enter`
  restores to the original location — or Inbox if its project no longer
  exists — `d` permanently deletes after a second `d` confirms via a
  footer line, no modal). Past its retention window (`:config` → General
  → "Trash retention", default 30 days, presets 7/14/30/60/90), a trashed
  note is purged automatically the next time pkm starts — no background
  timer, mirrors the existing startup index-validation scan.
  `internal/vault/trash.go` (`Trash`, `ListTrash`, `Restore`,
  `PurgeExpired`, `RemoveTrashEntry`), `internal/tui/trash_view.go`,
  `internal/tui/commands.go` (`cmdTrash`, `cmdDelete` updated),
  `internal/tui/appconfig.go` (`TrashRetentionDays`, config version 2).
  **The existing Ctrl+Z undo stack for `:delete` stays, unchanged in
  spirit** (locked decision: durable trash net + immediate undo net,
  not one replacing the other) — but undo/redo needed real surgery to
  stay correct once "delete" stopped meaning "gone": a generic
  Save-based undo would recreate the note while leaving an orphaned
  trash copy and sidecar entry behind (the note existing twice), and a
  generic Save-based redo would recreate the note instead of re-trashing
  it. `undoRecord` gained an `isDelete` flag; `handleUndo` now also calls
  `RemoveTrashEntry` for a delete record, and redo of a delete routes
  through a new `redoDelete` that calls `vault.Trash` again (mirroring
  `cmdDelete`'s exact side effects) instead of the generic redo path.
  Root-caused from the qualification's own explicit "Nachtrag" —
  written into the spec before implementation started, not discovered
  the hard way.

### Verify
- `go test ./...` — clean, including `TestHeadlessDeleteThenUndo`
  unchanged and still green (an explicit requirement — undo's existing
  contract had to keep working exactly as before).
- `internal/vault/trash_test.go`: `TestTrashMovesFileAndRecordsSidecarEntry`,
  `TestTrashCollisionAppendsSuffix`,
  `TestRestoreReturnsNoteToOrigPathWithOldState`,
  `TestRestoreCollisionAtOrigPathUsesSuffixedName`,
  `TestRestoreFallsBackToInboxWhenProjectGone`,
  `TestPurgeExpiredOnlyRemovesPastRetention`,
  `TestPurgeExpiredNonPositiveRetentionFallsBackToDefault` (a `<=0`
  retention value must never mean "purge immediately"),
  `TestRemoveTrashEntryDeletesFileAndEntry`,
  `TestRemoveTrashEntryUnknownIDIsNoop`.
- `internal/tui/tui_test.go`: `TestHeadlessDeleteThenUndoLeavesTrashEmpty`
  (the orphan-copy trap), `TestHeadlessDeleteUndoRedoReTrashesExactlyOnce`
  (the "Nachtrag" redo trap — exactly one trash file, exactly one sidecar
  entry after undo→redo), `TestDefaultConfigTrashRetentionDaysIs30`,
  `TestFillConfigDefaultsSetsRetentionForOldConfig`,
  `TestApplyConfigItemChangesTrashRetentionDays`,
  `TestHeadlessTrashCommandListsAndRestores`,
  `TestHeadlessTrashPermanentDeleteRequiresConfirm`,
  `TestHeadlessTrashOtherKeyCancelsConfirm`,
  `TestHeadlessTrashEmptyStateNoPanic`.
- Manual (tmux): created and deleted a note — file confirmed moved into
  `.pkm/trash/` with a matching sidecar entry (inspected both directly on
  disk); `:trash` listed it with "deleted today · 30 days left"; a single
  `d` showed the footer confirm without deleting, a different key
  cancelled it, a second `d` permanently deleted it (file and sidecar
  entry both gone); `Enter` on a second trashed note restored it (file
  back at its original path, sidecar entry cleared) — confirmed via an
  accidental double-Enter during testing, which doubled as an unplanned
  but valid end-to-end check. Cycled "Trash retention" in `:config` from
  30 → 14 days, saved, confirmed `trash_retention_days: 14` written to
  `config.yaml`. Backdated a trashed entry's `deleted_at` past the new
  14-day retention, restarted the app, confirmed it was purged
  automatically on startup with no timer needed. Separately, deleted a
  note and pressed `Ctrl+Z` immediately — confirmed the note reappeared
  at its original path *and* the trash file/sidecar entry were cleaned
  up, not left behind as an orphan.
- README ("Trash" section, Configuration table, vault structure tree,
  `:delete`/`:trash` command table rows), `help_view.go` (TRASH section,
  command list), `.gitignore` (`.pkm/trash/`, `trash.json`) updated.

### Added (issue batch, 2026-07-22, #2)
- **Text selection + copy in the Note Viewer** — `Shift+↑↓←→` extends a
  selection from the block cursor (anchor set on the first shift-move);
  `Ctrl+A` selects the whole note; `Ctrl+C` copies it via the existing
  OSC 52 mechanism (`copyToClipboardCmd`, already used by code-block copy)
  and reports the character count in the status bar, since OSC 52 silently
  truncates past a per-terminal size limit that a large `Ctrl+A` copy could
  hit unnoticed otherwise. `Esc` clears the selection first, then falls
  back to the usual "go back" on a second press. Any plain (non-shift)
  cursor movement clears it too. Mouse click-drag over plain body text
  (not a link/checkbox/heading, which keep their existing click behavior
  unchanged) selects from press to release and **auto-copies on release**
  — no `Ctrl+C` needed for a mouse selection, matching how selection works
  in every other terminal app. **No `Ctrl+V`** — dropped per the locked
  qualification decision, since view mode has no editable buffer to paste
  into; paste stays out of scope until #4 gives the editor a real
  selection/paste buffer.
  `internal/tui/viewer.go` (`selAnchorRow`/`selAnchorCol`/`selActive`/
  `dragging` fields, `moveCursorChar`, `selectedRawText`, selection
  highlighting in `withCursorOverlay`), `internal/tui/model.go`
  (`handleMouseDrag`, `handleMouseRelease`, drag-start branch in
  `handleMouseClick`, `Ctrl+C` dispatch-order fix below).
- **Copied text is always raw Markdown, never rendered ANSI output or a
  resolved `[[link|alias]]`.** Selection endpoints live in *rendered*-line
  coordinates, but the clipboard payload has to be the note's actual
  source — so copy reuses #22's rendered→raw line map (`rawLineAt`)
  instead of building a second one, exactly as the qualification
  specified. That map is line-level, not character-exact (see its own
  doc comment), so a selection's Ctrl+C/auto-copy is the **full raw
  lines** its start and end rendered rows touch, not an exact character
  span within them — the on-screen highlight uses the same whole-line
  granularity, so what's highlighted always matches what gets copied.
- **`Ctrl+C` dispatch-order fix.** The existing global "Ctrl+C always
  means Esc" remap sits at the very top of `Update()`'s `tea.KeyMsg`
  handling, before any mode sees the key. The selection-copy special case
  has to be checked *before* that remap runs, not after, or it can never
  reach the viewer — same class of bug as the Ctrl+L dispatch-order trap
  in #10's line operations. Fixed by gating the remap on
  `viewNote && selActive` instead of applying it unconditionally; every
  other Ctrl+C behavior (cancel editor, close overlay, etc.) is
  unchanged.

### Verify
- `go build ./...`, `go vet ./...`, `go test ./...` — clean.
- `internal/tui/tui_test.go`:
  `TestHeadlessMouseMotionWithoutDragDoesNotSelect` (motion alone, with no
  preceding press, must not start or extend a selection),
  `TestHeadlessSelectionShiftArrowExtendsAndPlainMoveClears`,
  `TestHeadlessSelectionCtrlASelectsEverything`,
  `TestHeadlessSelectionCtrlCWithoutSelectionActsLikeEsc` (guards the
  dispatch-order fix from regressing back to the unconditional remap),
  `TestHeadlessSelectionCtrlCCopiesRawTextWithCharCount` (also asserts the
  copied text contains no `\x1b` ANSI escapes),
  `TestHeadlessMouseDragSelectReleaseCopiesOnce` (press → drag → release
  copies exactly once; a second release after the drag ended is a no-op).
- Manual (tmux): typed a three-paragraph note via the in-app editor;
  `Shift+Down` × 3 visibly grew a highlighted block across multiple
  rendered rows (confirmed via `tmux capture-pane -e`, raw ANSI, since
  plain-text capture strips the highlight); `Ctrl+C` after 3×`Shift+Down`
  reported "copied 32 characters", matching `len("Alpha line
  one.\n\nBravo line two.")` exactly; `Ctrl+A` then `Ctrl+C` reported
  "copied 53 characters", matching the full three-paragraph body exactly;
  `Esc` with a selection active left the note open (selection cleared,
  no navigation); a second `Esc` then returned to the list. The
  "copied N characters" status text was invisible at a 110-column
  terminal width — traced to the pane's hotkey-hint bar already filling
  the width in Note Viewer context, truncating anything appended after it
  (`fitTooltipBar`); confirmed cosmetic-only, not a logic bug, by
  widening to 220 columns and seeing the same state correctly reported.
  **Not verified manually:** actually pasting the OSC 52 payload into
  another application — tmux's mouse protocol isn't practical to drive
  synthetically for the press/drag/release path either, so that specific
  flow relies on `TestHeadlessMouseDragSelectReleaseCopiesOnce` above
  rather than an end-to-end terminal check.
- README (`Ctrl+C` global-rules caveat, new "Text Selection" subsection,
  mouse-support line), `help_view.go` (new `TEXT SELECTION` section)
  updated. `Ctrl+V` does not appear anywhere in either.

### Added (issue batch, 2026-07-22, #3)
- **Bottom hotkey bar groups chips by prefix, mode-dependently ordered.**
  The tooltip/chip bar (`Model.renderTooltipBar`) already grouped Shift and
  Ctrl shortcuts under `SHIFT +`/`CTRL +` labels, but always in the same
  fixed order regardless of mode, with the unbound `:`/`?` keys shown
  first. Now the primary group leads: view mode (the default bar, covering
  List/Note Viewer/Trash/Project Detail — everything but Edit and the
  modal overlays) leans on Shift shortcuts and shows `SHIFT +` first, then
  `CTRL +`, then the unbound keys last; Edit mode leans on Ctrl shortcuts
  and shows `CTRL +` first, then its unbound `Tab`/`Esc` chips last (it has
  no Shift group of its own). No chip text, action label, or the set of
  chips shown changed — purely a reordering/regrouping, per the locked
  "chirurgische Änderung" requirement.
- **Narrow-width fallback drops the group labels.** Below a new
  `tooltipGroupMinWidth` (80 columns), `labeledGroup` renders just the
  chips with no `SHIFT +`/`CTRL +` header text — the label itself was the
  thing that could eat more space than it was worth at that width, not the
  grouping logic, so only the label text is what's conditionally dropped;
  the existing hard-truncation in `fitTooltipBar` still guarantees the bar
  never wraps to a second line, at any width.
- `help_view.go`'s static reference already had SHIFT SHORTCUTS/CTRL
  SHORTCUTS sections; verified every chip shown in the live bar is listed
  there too, and found one gap — `?` (toggle Help) was never documented
  anywhere in the reference page itself — added to `NAVIGATION`. Left the
  static SHIFT-before-CTRL section order unchanged: the reference is one
  flat page, not mode-aware like the live bar, and that order already
  matches View mode (the default/primary context for this page).

### Verify
- `go build ./...`, `go vet ./...`, `go test ./...` — clean; final
  `go build -o pkm ./cmd/pkm`.
- `internal/tui/tui_test.go`:
  `TestHeadlessTooltipBarViewModeGroupsShiftBeforeCtrl`,
  `TestHeadlessTooltipBarEditModeGroupsCtrlFirst`,
  `TestHeadlessTooltipBarNarrowWidthDropsGroupLabels` (labels gone below
  the threshold, chips still there), `TestHeadlessTooltipBarNeverExceedsWidth`
  (widths 40–220, view and edit mode — the bar's rendered width never
  exceeds the pane width).
- Manual (tmux): captured the bar at 150 columns in view mode (`SHIFT +`
  group, then `CTRL +`, then `:`/`?` last) and in Edit mode (`CTRL +`
  group, then `Tab`/`Esc` last); captured at 60 columns and confirmed
  both group labels were absent while the leading chips (`N`, `A`, `D`,
  ...) still rendered.
- README (`### Global` section: new paragraph on the bar's grouping and
  narrow-width fallback) updated.

Remaining backlog tracked as GitHub issues (repo `ruttkowa/pkmcli`):
GitLab Issues integration (#15) — the only issue in this cycle's
qualified list not attempted; deferred as too large for this batch's
autonomous session budget, per the note left at the start of this batch.

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
