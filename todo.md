# Todo / Session-Resumption Ledger

This file is the source of truth for in-flight work across session
boundaries (context clears, session limits). Read it first when resuming.

**Rules for updating this file:**
- Status markers: `[ ]` not started · `[~]` in progress · `[x]` done.
- Every task keeps an **Implementation notes** block. Update it every time
  work happens on that task — what's done, what's left, exact file/line
  pointers, any decision made and why. Write it so a fresh session with no
  memory of this conversation can resume in one read.
- When a task is finished: mark `[x]`, add it to `CHANGELOG.md` under
  `[Unreleased]` (or cut a new version section — see memory `versioning`
  convention), list the tests/manual steps that verify it, then remove it
  from `[Unreleased]` into the versioned section.
- Order below is dependency order, not request order — see "Sequencing"
  note on item 5 and item 7.

---

## 1. [x] Import existing markdown file into the vault

**Request:** `:import` command or hotkey opens a popover dialog: a file
path field with autocomplete/dynamic suggestions, a toggle (Space) for
move-vs-copy (default: move), and a destination-state field (default:
Inbox). On confirm, the file is moved (or copied) into `vault/notes/`,
renamed to the `<ID> <Title>.md` convention, and given frontmatter.

**Open questions / decisions to make when starting:**
- Title source: derive from the file's first H1, or from the filename?
  (Recommend: filename minus extension, since arbitrary markdown may have
  no H1 — but confirm with user or pick and note the choice here.)
- Path autocomplete: reuse whatever path-completion exists for `:config
  export/import` (check `internal/tui/commands.go` cmdConfigExport/Import
  and how those resolve paths) rather than writing a new completer.
- What happens to existing frontmatter in the imported file, if any?
  (Recommend: preserve tags if a `tags:` key exists, but always assign a
  fresh `id`/`created`/`updated`/`state` — imported files won't already
  match this app's schema.)

**Implementation notes:** done 2026-07-08. Resolved the open questions as
recommended above (no separate path-completion pattern existed to reuse —
`:config export/import` just takes a plain string arg, no completer — so
built a small `pathSuggestions()` directory-listing helper from scratch;
title = filename minus extension; existing `tags:` preserved via
`parseNote` into a fresh `*Note`, then `ID`/`Title`/`Created`/`Updated`/
`State`/`Path` are always overwritten).

- `internal/vault/vault.go`: added `Vault.Import(srcPath, state, move)` —
  reads the source, runs it through the existing `parseNote` (frontmatter
  parser) so any `tags:` survive, stamps fresh ID/title/timestamps/state,
  `Save`s to the vault, then `os.Remove`s the source iff `move`.
- `internal/tui/import_pane.go` (new file): `importPane` popover model,
  structured like `configPane` (`internal/tui/config_pane.go`) — own
  `update`/`render`, focus-stop enum (`impFldPath` → `impFldMove` →
  `impFldDest` → `impFldConfirm`), `Tab`/`Shift+Tab` cycles fields (same
  pattern as item 2). `pathSuggestions()` lists dirs + `.md` files in the
  fragment's directory, prefix-filtered, capped at 6, live on every
  keystroke. Destination field reuses `renderStateSelector` from
  editor.go (no new widget). Confirm is a distinct focus stop (a
  "[ Import ]" button) rather than overloading Enter, since Enter already
  means "cycle value forward" on the Destination field and "accept
  suggestion" on Path — avoided that ambiguity by giving Confirm its own
  stop, consistent with there being no single global "submit" gesture in
  this popover.
- Wired into `Model` (model.go): `showImport bool` + `importView
  importPane` fields, key-guard block right after the `showConfig` guard
  (same early-return pattern), render dispatch (replaces the main pane
  the same way config does), tooltip-bar hint. `I` hotkey opens it
  directly (bypasses the palette, like `?` does for help) since there's
  no useful "prefill" for a popover. `:import [path]` command
  (commands.go `cmdImport`) opens the same popover with the path
  pre-filled if given.
- `Model.runImport()` (commands.go): reads `m.importView`, calls
  `vault.Import`, and on success does the same bookkeeping as `:new`
  (`index.Upsert`, `titleSet` entry, open the note in the active split,
  `refreshCounts`). On error, sets `importView.errMsg` and leaves the
  popover open so the user can fix the path and retry — does not reset
  `confirmed` incorrectly (checked: `confirmed` is set back to `false` in
  the error path so Enter doesn't get "stuck" re-submitting).
- Verified: `go test ./...`, new vault-level tests (`TestImportMoves...`,
  `TestImportCopies...`, `TestImportPreservesExistingTags...`) and
  TUI-level tests (`TestHeadlessImportPopover`,
  `TestHeadlessImportPopoverEsc`) — see CHANGELOG `[Unreleased]` Verify
  list for exact names. Manual tmux pass: full flow from `I` through a
  real file on disk, both Move and Copy checked (Copy checked live;
  Move covered by the vault-level test + confirmed by inspection).
  Docs updated: `README.md` (new "Import Popover" section, hotkey table,
  All Commands table), `internal/tui/help_view.go` (new IMPORT POPOVER
  section, command list, shift-shortcuts line).

**Not done / deferred:** destination-state autosuggest beyond the
existing left/right cycle (e.g. typing to filter states) — not requested,
five states cycle fine with arrows. No dedicated `import_pane_test.go`;
tests live alongside the existing `vault_test.go` / `tui_test.go` files,
matching this codebase's convention of one test file per package rather
than per feature.

---

## 2. [x] Shift+Tab pane cycling, and audit every existing Tab-toggle site

**Request:** `Shift+Tab` cycles panes backward, mirroring `Tab` forward.
Apply the same forward/backward pattern anywhere Tab currently toggles
between items (not just the pane switcher).

**Known Tab sites to check (enumerate fully before touching):**
- Global pane focus cycle (`Tab` — grep `case "tab"` in model.go for the
  top-level handler).
- `:config` view — Tab cycles General / Keybindings / Variables sections.
- Palette autocomplete — Tab completes the current suggestion. Shift+Tab
  here should NOT be assumed safe; check whether it should reverse-cycle
  suggestions or is a no-op/conflicts with terminal behavior. Decide and
  note the reasoning here before implementing.
- Any other `tea.KeyTab` / `"tab"` string match in the codebase — grep
  `grep -rn '"tab"\|KeyTab\|KeyShiftTab' internal/tui/` before starting.

**Implementation notes:** done 2026-07-08. Audit found most sites already
handled Shift+Tab from prior sessions: editor field-cycling
(`editor.go:242`, `cycleField(-1)`) and `:config` section switcher
(`model.go:376`, `prevSection()`) both already had it. Only the global
Sidebar↔Main toggle (`model.go:527`) was missing it — added `"shift+tab"`
to that case alongside `"tab"` (binary toggle, so both directions produce
the same result, which is fine — matches the literal request). Palette
Tab-complete (`palette.go:530`) reviewed and intentionally left alone: it
completes text, it doesn't cycle a list (up/down already do that), so
there's no natural "reverse" behavior to bind. Split-cycling
(`m.cfg.Keymap.NextPane`, forward-only, default `ctrl+w`) has no
backward companion and wasn't touched — out of scope, user's request was
specifically about "navigation and editor/viewer pane" (the sidebar/main
toggle), not multi-split cycling; flag if the user wants that too.
Verified: `go test ./...` green, plus new assertions in
`TestHeadlessNavigation` (tui_test.go) exercising Shift+Tab both
directions. Not manually tmux'd (pure key-routing change, no rendering
change, fully covered by the headless test).

---

## 3. [x] :import destination picker reuses existing state-selection pattern

(Folded into item 1, done alongside it 2026-07-08 — not a separate task.
Reused `renderStateSelector` from editor.go rather than inventing a new
widget; see item 1's Implementation notes for the actual detail.)

---

## 4. [ ] Soft-delete / trash with 30-day retention + recovery

**Request:** Deleting a note moves it to a hidden/separate space instead of
permanently removing it immediately. Kept 30 days, then actually deleted.
Configurable retention window in `:config`. `:config` also gets a way to
list/recover deleted notes; recovery restores to original location or to
Inbox.

**Supersedes:** the current `cmdDelete` in `internal/tui/commands.go`
(added 2026-07-07/08, documented in CHANGELOG 0.1.0) which does a hard
`os.Remove` + undo-stack safety net. That undo-stack approach only
survives until the user does 20 more undo-able actions (stack cap) or
quits the app (stack is in-memory, not persisted) — the new trash must be
a durable, disk-based replacement, not an addition on top.

**Design sketch (not yet validated with user):**
- Move file to `vault/.pkm/trash/<id> <title>.md` (or similar) instead of
  `os.Remove`. Keep frontmatter as-is, add a `deleted_at` timestamp
  (either as a new frontmatter field or a sidecar/index record — decide
  based on whether frontmatter schema changes are acceptable, since
  spec.html defines the frontmatter shape).
- A background/startup sweep (mirrors the existing index validation scan
  on startup — see CLAUDE.md "Index Architecture") purges anything past
  the retention window.
- `:config` needs: a numeric "trash retention (days)" setting (add to
  `AppConfig` in `internal/tui/appconfig.go`, bump `currentConfigVersion`
  per that file's existing versioning convention), and a recovery list UI
  (new view, or a section of an existing overlay — decide during
  implementation).
- Interaction with existing undo/redo stack: once this lands, does
  `:delete` still push an undo record, or does undo become unnecessary
  because trash IS the undo? (Recommend: drop the undo-stack record for
  delete once trash exists, since trash supersedes it — but confirm this
  doesn't break `TestHeadlessDeleteThenUndo` semantics; that test will
  need rewriting to check trash-and-recover instead of undo-and-recreate.)

**Implementation notes:** not started. This is the task most likely to
need a user check-in before coding, given the frontmatter-schema and
undo-stack-interaction questions above.

---

## 5. [ ] Text selection (mouse / Shift+Arrow), copy (Ctrl+C), paste (Ctrl+V), select-all (Ctrl+A) — VIEW MODE ONLY

**Request:** Highlight text via mouse drag or Shift+Arrow. Mouse-drag
selection auto-copies to clipboard on mouse release (keyboard selection
does NOT auto-copy — only explicit copy). Ctrl+C copies, Ctrl+V pastes,
Ctrl+A selects all.

**Scope decision (user, 2026-07-08):** view-mode only. `bubbles/textarea`
has no selection API (confirmed via
`grep -n "Selection\|CursorRow\|CursorColumn" .../bubbles@v1.0.0/textarea/textarea.go`
— no matches), and forking it is a ~1500-line undertaking, same order as
the heading-styling fork rejected last session. The user chose to skip
edit-mode selection entirely for now rather than fork or reduce scope —
**edit mode keeps no text-selection feature.** The fork itself, and every
feature that would benefit from it, is tracked separately as item 8 below
— do not fold that work back into this item.

**Sequencing: do this LAST of the feature work (before item 7).**

**Other constraints found during triage:**
- Global `Ctrl+C` currently means "cancel", same as Esc, in every mode
  (`internal/tui/model.go` ~line 266, remaps ctrl+c → KeyEsc; documented
  in help_view.go CTRL SHORTCUTS section and README). Reasonable
  resolution: Ctrl+C copies the active selection if one exists, otherwise
  falls back to cancel — but this is a behavior change worth flagging
  alongside the selection-scope question above, not deciding unilaterally.
- Copy is easy (existing OSC 52 path via `go-osc52`, see
  `copyToClipboardCmd` in viewer.go — write-only, SSH-safe). **Paste is
  not symmetric**: reading the system clipboard needs either an OS-native
  clipboard library (breaks over SSH/tmux the way OSC 52 doesn't) or
  terminal bracketed-paste support. Verify which is feasible in this
  Bubble Tea setup before promising Ctrl+V in the changelog — don't ship
  a paste that silently does nothing over SSH without at least a status
  message explaining why.
- Mouse drag-select needs `tea.MouseActionMotion` events, which the app
  currently ignores by design (see `TestHeadlessMouseMotionIgnored` in
  tui_test.go). **Checked 2026-07-08:** `cmd/pkm/main.go:58` already uses
  `tea.WithMouseCellMotion()`, which (per Bubble Tea semantics) reports
  motion events while a button is held — i.e. drag events do arrive today,
  the app just discards them. So this is lower-risk than it looked: no
  `tea.NewProgram` option change needed, just start handling
  `MouseActionMotion` while a "selecting" flag is active.
- **Unresolved contradiction, flag to user before building paste:** with
  edit-mode selection out of scope, view mode has no editable buffer to
  paste *into* — `Ctrl+V` has no natural target under this scope. Options
  once this is picked back up: (a) drop Ctrl+V from this item entirely and
  revisit it only alongside item 8 (edit-mode fork), (b) `Ctrl+V` while a
  note is open jumps into the editor and pastes at the cursor (blurs the
  "view mode only" boundary but gives the key somewhere to go), (c) some
  other target the user has in mind. Don't guess — ask before implementing
  paste specifically; copy/select-all/select-via-drag-or-shift-arrow have
  no such ambiguity and can be built without waiting on this.

**Implementation notes:** not started. Design sketch for when this is
picked up: extend `viewerModel`'s existing `cursorRow`/`cursorCol` (from
the block-cursor work) into a `selAnchor`/`selActive` pair; highlight the
spanned rows/columns in `withCursorOverlay` the same way link/checkbox/
code-span highlighting already works; Shift+Arrow extends the selection,
any non-shift movement or Esc clears it; `Ctrl+A` sets the anchor to
document start and cursor to document end; `Ctrl+C` copies the selected
text via the existing OSC 52 `copyToClipboardCmd` path (and, per the
triage above, should fall back to today's cancel behavior when no
selection is active, but confirm that fallback with the user since it's
a behavior change to a documented global shortcut). Mouse-drag: track a
"dragging" flag from `MouseActionPress` to `MouseActionRelease`,
updating the selection on each `MouseActionMotion` in between, and fire
the OSC 52 copy on release. Next step when resuming: build copy/select-
all/drag/shift-arrow selection in the viewer per this sketch, and raise
the Ctrl+V question above before touching paste.

---

## 6. [x] Move "saved" status out of the hotkey toolbar into the document pane footer

**Request:** Remove "saved" status from the hotkey/tooltip bar. Show it
separately, fixed at the bottom of the document pane: "Last saved at:
<timestamp>" plus an indication of unsaved changes.

**Sequencing: good first task to pick up — self-contained, no
dependencies on other items in this file.**

**Where to look:** the current save-status display is presumably driven
by `m.statusMsg = "saved: " + n.Title` (set in multiple places in
model.go/commands.go) and rendered wherever the tooltip/hotkey bar is
built. Need: (1) a per-note or per-split "last saved at" timestamp field
(likely on `splitPane` or on the note itself via `Note.Updated`, which
already exists per spec.html frontmatter — check if `Updated` can just be
reused directly instead of adding new state), (2) a "dirty" / unsaved-
changes flag (check if editor already tracks a dirty bit for the draft;
if not, compare current `ta.Value()` against last-saved body), (3) a
rendering spot fixed at the bottom of the viewer/editor pane, separate
from the hotkey bar.

**Implementation notes:** done 2026-07-08. Reused `Note.Updated`
(existing frontmatter field, stamped by `vault.Save`) for the timestamp —
no new persisted state needed. Viewer footer: extended the existing
scroll-percentage row in `viewer.go render()` (already "fixed at the
bottom" of the pane) to show `Last saved: HH:MM:SS` left-aligned, pct
right-aligned, same row. Editor footer: extended the existing word/line
count row in `editor.go render()` the same way, plus a new
`editPane.dirty()` method comparing the live draft (body, tags — via
`parseTags`, project, state) against `e.note`'s last-saved values (that
pointer is untouched until commit, so the comparison is exact — no
separate dirty flag/snapshot needed). Removed the one-off
`m.statusMsg = "saved: " + n.Title` in `commitEditorDraft` (model.go) —
that was the only site setting it (checked via grep). Left `save error:`
in the global toast as-is (an error, not "the saved status" per the
request's literal wording); flag if the user wants errors moved too.
Verified: `go test ./...`, new `TestEditorFooterShowsDirtyAndSavedTime`
(tui_test.go — checks dirty marker on/off, timestamp text, and that
`m.statusMsg` stays empty after save), plus manual tmux pass: edited a
note, saw "● Unsaved changes" appear, `Ctrl+S`, marker cleared and
timestamp advanced, bottom hotkey bar had no leftover "saved:" text;
`Ctrl+Z` after to revert the test edit.

**Bug found and fixed in a second tmux pass (advisor caught the blind
spot: first pass only tested a single wide pane):** the initial footer
implementation used a bare `lipgloss...Width(width).Render(...)`, which
pads short content but does **not** truncate long content — in a `:split`
with the editor open in a narrower pane, the footer text (with the
"● Unsaved changes" marker) exceeded the pane width and wrapped onto a
second line, which pushed the pane's rendered content past
`calcBodyHeight`'s `editFooterRows=1` reservation and desynced the
bordered box's height (visible as the border closing above the wrapped
line). Fixed by adding `editPane.footerText(width, totalLines)`
(editor.go) — builds progressively shorter candidates (full → drop the
"Tab: cycle fields" hint → shrink the dirty marker to just "●" → drop
word/line counts too) and picks the first that fits `lipgloss.Width`,
with `xansi.Truncate` as a hard safety net. Viewer got the equivalent
`footerRow(width, left, right)` helper (viewer.go) for its "Last
saved / pct" row (drops "Last saved: HH:MM:SS" entirely and right-aligns
just the percentage if there's no room). Locked in with
`TestFooterRowNeverExceedsWidth` and a width-sweep loop added to
`TestEditorFooterShowsDirtyAndSavedTime` (both check every candidate
width stays ≤ the pane width, down to 0), plus reproduced-then-confirmed
live in tmux (`:split`, open a note in the second pane, `e` — footer now
renders on one line).

---

## 7. [ ] Group hotkeys in help/tooltip UI: Ctrl+, Shift+, and plain, ordered by mode

**Request:** In the hotkey reference / tooltip bar, group commands under
"STRG/CTRL + …", "SHIFT + …", and plain-key headers. View mode should list
Shift-group first (view mode leans on Shift shortcuts); edit mode should
list Ctrl-group first (edit mode leans on Ctrl shortcuts).

**Sequencing: do this LAST of everything in this file.** It depends on the
final hotkey set from items 2 (Shift+Tab), 4 (soft-delete: does hotkey set
change? recovery hotkey?), 5 (Ctrl+C/V/A, Shift+Arrow), and 6 (does
removing "saved" from the tooltip bar change its layout enough to affect
this grouping work?). Doing this first would mean redoing it once those
land.

**Where to look:** `internal/tui/help_view.go` (the full reference,
already has a SHIFT SHORTCUTS and CTRL SHORTCUTS section — this item is
about applying the same grouping to the live in-app tooltip/chip bar, not
just the `:help` page) and wherever `chip(...)` calls are assembled in
model.go for the bottom hotkey bar.

**Implementation notes:** not started.

---

## 8. [ ] Fork/vendor `bubbles/textarea` — deferred, no timeline

**Not requested yet — created 2026-07-08 per explicit user instruction**
when scoping item 5, to hold the fork question and everything blocked on
it in one place instead of re-discovering the same wall each time a
feature needs per-token editor rendering. Do not start this speculatively;
wait for the user to prioritize it.

**Why this exists:** `bubbles/textarea` (pinned v1.0.0) has no selection
API and no per-token/inline styling hook — only whole-row styling
(cursor-line vs. other) plus the `SetPromptFunc` gutter hook already used
for the heading marker. Two separate features have now hit this same
ceiling:
1. **Edit-mode heading styling** (2026-07-07/08 session) — wanted a
   whole-line accent for headings; landed on a cheap gutter-marker (`▎`)
   instead because a real per-line/per-token style needs the fork.
2. **Edit-mode text selection** (item 5, 2026-07-08) — wanted mouse-drag
   / Shift+Arrow selection with copy; landed on view-mode-only because
   selection needs the fork.

**If this is ever picked up:** it unblocks both of the above at once (do
them together, not separately) — real per-line heading accents, and full
edit-mode text selection/copy matching item 5's spec. Estimate ~1500
lines based on `bubbles@v1.0.0/textarea/textarea.go`'s size at the time
of this note; re-check that estimate against whatever version is vendored
when this is actually started, library internals may have changed.
Decide fork vs. vendor-and-patch vs. upstream-contribution before
starting — not evaluated yet.

**Implementation notes:** not started; this entry only exists to park the
decision and its blocked features.

---

## Versioning & Changelog process (see memory for the durable version of this)

- SemVer, pre-1.0. MINOR bump per feature (or feature-batch) landing,
  PATCH bump for fix-only work. Baseline `0.1.0` = everything through
  2026-07-08 (block cursor, search, delete+undo, mouse wheel, sidebar
  glyph-click fix, heading styles, fence-autoclose fix).
- Every completed task in this file gets a `CHANGELOG.md` entry with an
  explicit **Verify** list (test names + one manual step), before being
  marked `[x]` here.
- No `--version` flag / build-time version string — not requested, don't
  add speculatively.
