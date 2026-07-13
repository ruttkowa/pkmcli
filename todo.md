# Todo / Session-Resumption Ledger

> ## ⚠️ SUPERSEDED — active todos now live in GitHub Issues
>
> As of **2026-07-09**, task tracking moved to the GitHub repo
> **`ruttkowa/pkmcli`** Issues. **Do not add or track open work here anymore.**
> All 15 open items below were migrated to Issues (labeled
> `generated_by_claude`). Human-written issues carry `to_qualify_by_claude`
> and must be reviewed/clarified by Claude before work.
>
> This file is kept **only** as historical implementation notes for
> already-completed items (1, 2, 3, 6). Migration map (todo item → issue):
>
> | Item | Issue | Item | Issue | Item | Issue |
> |------|-------|------|-------|------|-------|
> | 4 | [#1](https://github.com/ruttkowa/pkmcli/issues/1) | 10 | [#6](https://github.com/ruttkowa/pkmcli/issues/6) | 16 | [#12](https://github.com/ruttkowa/pkmcli/issues/12) |
> | 5 | [#2](https://github.com/ruttkowa/pkmcli/issues/2) | 11 | [#7](https://github.com/ruttkowa/pkmcli/issues/7) | 17 | [#13](https://github.com/ruttkowa/pkmcli/issues/13) |
> | 7 | [#3](https://github.com/ruttkowa/pkmcli/issues/3) | 12 | [#8](https://github.com/ruttkowa/pkmcli/issues/8) | 18 | [#14](https://github.com/ruttkowa/pkmcli/issues/14) |
> | 8 | [#4](https://github.com/ruttkowa/pkmcli/issues/4) | 13 | [#9](https://github.com/ruttkowa/pkmcli/issues/9) | 19 | [#15](https://github.com/ruttkowa/pkmcli/issues/15) |
> | 9 | [#5](https://github.com/ruttkowa/pkmcli/issues/5) | 14 | [#10](https://github.com/ruttkowa/pkmcli/issues/10) | | |

The historical notes below are the source of truth only for the
**completed** items' implementation detail. Active work: see GitHub Issues.

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

## 9. [ ] Editor: Esc must SAVE, never silently discard — plus undo history

**Request:** "There are options to exit the edit mode and nothing is saved.
This must be prevented at any time. Exiting should save the document but
also keep an undo history."

**The bug, exact locations:** in the editor, `Esc` sets
`e.cancelled = true` (`editor.go:313-315`). `model.go:342-345` consumes
that flag by **throwing the draft away** (`sp.editor = editPane{}`;
`sp.activeView = viewNote`) — no `vault.Save`, no index update. So every
Esc-out of the editor loses all typed changes. (The configured save key,
default `Ctrl+S`, is the only path that commits, via `sp.editor.saved` →
`m.commitEditorDraft(sp)` at `model.go:340-341`.) That is the whole
"nothing is saved" complaint.

**Fix (recommended):** make Esc commit instead of discard. Simplest change
that satisfies the request: in `model.go:342-345`, replace the
discard-branch with a call to `m.commitEditorDraft(sp)` (same function Save
uses), so leaving the editor by ANY route saves. Keep the flag name or
rename `cancelled`→`exited` for clarity (surgical: renaming touches
`editor.go:314` and the two `model.go` read sites — grep `cancelled` in
`internal/tui/` first, `editor.go:288-315` also references it).

**Undo history — check what already exists before adding anything:**
`commitEditorDraft` (`model.go:162`) already pushes an `undoRecord` onto
`m.undoStack` (same stack `applyCheckboxToggle` uses at `model.go:992`,
capped at 20, cleared-redo). So routing Esc through `commitEditorDraft`
gives undo **for free** — Ctrl+Z after an Esc-save reverts to the
pre-edit note. VERIFY this is true by reading `commitEditorDraft` in full
before writing new undo code; do NOT add a second undo mechanism if the
existing stack already covers it (Simplicity First). The only gap to
confirm: if the draft is unchanged (`!e.dirty()`, `editor.go:117`), skip
the save+undo push entirely so a no-op open/close doesn't spam the undo
stack — check whether `commitEditorDraft` already guards on `dirty()`; if
not, add that guard.

**Tension to flag (CLAUDE.md):** the project charter says "not a text
editor; editing is delegated to an external editor." Auto-save-on-exit +
undo history push the inline editor further toward being a real editor.
The project has already drifted this way (block cursor, fence autoclose,
list continuation), so this is likely fine — but surface it to the user at
pick-up so they reconcile the charter rather than discovering the drift
mid-build. Related deferred work: item 8 (textarea fork).

**Open question:** should there be ANY "discard changes" escape hatch
(e.g. a `:revert` command, or a confirm-discard prompt on Esc)? The
request says "prevented at any time," which argues for no discard path at
all — recommend shipping save-always with Ctrl+Z as the only "undo,"
and add `:revert` later only if asked. Note the decision here when taken.

**Implementation notes:** not started. Verify at pick-up: type into a
note, press Esc, reopen the note → text is present; Ctrl+Z → reverts.
Add a headless test mirroring `TestHeadlessDeleteThenUndo` (tui_test.go):
open editor, mutate body, send Esc, assert `vault.Load` shows the change
AND `m.undoStack` grew by one. Manual tmux pass required (per memory
`manual_verify_tui`).

---

## 10. [ ] Editor: Shift+Enter = plain newline that BREAKS list continuation

**Request:** "Shift + Enter does not break up auto dashes or to-do boxes if
current line has a to-do box / dash — but it should. Auto-add another dash
or to-do box in a new line should only work with Enter."

**Current behavior, exact locations:** `updateBody` (`editor.go:366`) has a
`case "enter":` (`editor.go:418-421`) that calls `handleEnter()`
(`editor.go:537-561`), which auto-continues the list: on a `- [ ] `/`- [x] `
line it inserts `\n- [ ] `; on `- ` → `\n- `; on `* ` → `\n* `; on a
numbered line → next number. There is **no** `"shift+enter"` case, so
Shift+Enter falls through to `e.ta.Update(km)` at `editor.go:425` and the
underlying `bubbles/textarea` does whatever it does with it (likely a bare
newline, possibly nothing — verify in tmux which).

**Fix (recommended):** add an explicit `case "shift+enter":` in the
`updateBody` switch (near `editor.go:418`) that inserts a plain `"\n"` with
NO continuation — i.e. `e.ta.InsertString("\n")` (or route to
`e.ta.Update(km)` if that already yields a clean newline) and update
`e.wordCount`. This makes Enter = continue-list, Shift+Enter = break-out,
exactly as requested. Keep `handleEnter` untouched.

**Watch out:** confirm the terminal actually delivers a distinct
`"shift+enter"` KeyMsg in this Bubble Tea setup — some terminals collapse
Shift+Enter into plain Enter and it never arrives as a separate key. Test
in tmux FIRST (`grep` for any existing `shift+enter` handling — there is
none today). If the terminal can't distinguish it, tell the user and offer
an alternative gesture (e.g. Enter-on-an-empty-continuation-line already
breaks out in many editors: pressing Enter on a `- [ ] ` line that has no
text after the marker deletes the marker instead of adding another). Note
which path was taken.

**Implementation notes:** not started. Verify: on a `- [ ] ` line, Enter →
new `- [ ] ` line (unchanged); Shift+Enter → blank new line, no marker.
Same for `- `, `* `, numbered lists. Headless test in tui_test.go asserting
the resulting `ta.Value()` for both keys on each list type.

---

## 11. [ ] Viewer: multi-line ``` fenced code blocks styled like inline `code`

**Request:** "Code blocks marked with ``` do not highlight the same as
single-line code with `code`." Bring fenced-block rendering to parity with
inline code-span styling in the read-mode viewer.

**Where to look:** `viewer.go` — `buildBodyMd` (`viewer.go:519`) and
`processCheckboxesAndCode` (called at `viewer.go:114`) tag code and build
`m.codeSpans`; `codeSpanAt` / the cursor-overlay code-span highlight path
(`viewer.go:196-199`, `viewer.go:365-367`) handle **inline** `` `code` ``.
Determine how ``` fenced blocks currently render (whether they go through
glamour, a raw passthrough, or are untouched) and why their styling
diverges — read `processCheckboxesAndCode` and `buildBodyMd` fully before
changing anything.

**Open question / decision:** define what "the same as `code`" means
concretely — same background/foreground swatch (`activeTheme` code colors),
per-line vs. whole-block background, and whether fenced blocks should also
be cursor-selectable/copyable the way inline spans already are
(`pendingCodeCopy`, `viewer.go:197`). Recommend: match the inline code
swatch, apply it to every line of the fenced block (full-width background),
and — if cheap — extend the existing `codeSpans` copy affordance to whole
fences so Enter-on-a-fence copies the block. If copy-of-fence is more than
a small addition, split it out and do styling-only first.

**Implementation notes:** not started. Verify with a note containing both
an inline span and a ``` fenced block: both render with the same code
styling. Add a viewer render assertion (tui_test.go) checking the fenced
lines carry the code style. Manual tmux confirmation (render-only change —
memory `manual_verify_tui`).

---

## 12. [ ] Viewer bug: cursor jumps to top after toggling a checkbox

**Request:** "When toggling a todo item in view mode the cursor jumps back
to the top after toggling. It should stay where it is."

**Root cause — ALREADY PINNED, do not re-diagnose from scratch:** the
obvious-looking site, `applyCheckboxToggle` (`model.go:977-1004`),
**already** saves and restores `cursorRow/cursorCol/scrollOff`
(`model.go:999-1001`) and is NOT the bug. The real reset happens
asynchronously: `applyCheckboxToggle` calls `m.vault.Save(n)`
(`model.go:988`), which writes the `.md` file; the fsnotify watcher
(`watcher.go:39`) fires, reloads the note, and sends `vaultChangedMsg`;
the handler at `model.go:701-713` calls
`m.splits[i].viewer.withNote(msg.note)` (`model.go:709`), and `withNote`
resets `cursorRow = 0` / `cursorCol = 0` (`viewer.go:73`) with **no**
position restore. That async reload is what snaps the cursor to the top.

**Fix (recommended):** in the `vaultChangedMsg` handler (`model.go:705-712`),
preserve and restore `cursorRow/cursorCol/scrollOff` around the
`withNote`+`preRender` calls — the exact pattern already used in
`applyCheckboxToggle` at `model.go:999-1001`. Clamp the restored row to the
new line count (the note may have grown/shrunk). Consider factoring the
save→restore into a small helper reused by both sites, but only if it
stays simple (Surgical Changes: two call sites is borderline — a helper is
fine, a new abstraction layer is not).

**Verify the diagnosis first, then fix:** reproduce in tmux (open a note
with several checkboxes, scroll/cursor down, toggle one, watch the cursor
jump). If the jump does NOT reproduce, or reproduces via a different path,
say so in these notes rather than "fixing" what's already correct at
`model.go:999-1001`. Interaction with item 16 (finished-tasks-sink): if 16
lands first as a VIEW-layer reorder, "stay where it is" needs a tie-break
(cursor holds the line position vs. follows the toggled task) — resolve
that in 16; here, plain "hold position" is correct.

**Implementation notes:** not started. Headless regression test: set
`cursorRow`/`scrollOff` to nonzero, drive a checkbox toggle, feed the
resulting `vaultChangedMsg`, assert position unchanged. Manual tmux pass.

---

## 13. [ ] Viewer: Space toggles a to-do anywhere on the line

**Request:** "In view mode — if the line is a todo it should be marked done
with the Space key independently of where in the line the cursor is. The
cursor doesn't need to be inside the box. Only if the line starts with a
todo / qualifies as a todo."

**Good news — mostly already structured for this.** Toggle today is bound
to `Enter` (`viewer.go:248-249` → `activateCursor`, `viewer.go:187`), and
`activateCursor` already keys the checkbox off
`checkboxRawLineAt(m.cursorRow)` (`viewer.go:192`) — that is **line-based,
column-independent** already. So the request is essentially: bind `Space`
as an additional trigger for the checkbox branch (and only the checkbox
branch — Space should NOT open links or copy code spans the way Enter
does).

**Fix (recommended):** add a `case " ":` (or `tea.KeySpace`) in
`viewer.update` (`viewer.go:206`) that sets `pendingCheckboxRaw` iff
`checkboxRawLineAt(m.cursorRow)` returns ok, and is a no-op otherwise
(so Space over a non-task line does nothing surprising). The existing
dispatcher in `model.go:650-654` already consumes `pendingCheckboxRaw` and
calls `applyCheckboxToggle` — reuse it, don't duplicate. Keep Enter's full
behavior (links/code/checkbox) intact.

**Depends on item 12:** the cursor-jump fix must land first (or together),
or Space-toggling will feel just as broken as Enter-toggling does today.

**Open question:** does Space currently do anything else in the viewer
(page-down is a common default)? Grep `viewer.update` for an existing
space/`" "` handler (none seen in triage) and confirm no conflict before
binding. Note the finding.

**Implementation notes:** not started. Headless test: cursor on a task
line at a nonzero column, send Space, assert the checkbox flipped; cursor
on a plain line, send Space, assert no-op. Manual tmux pass.

---

## 14. [ ] Editor: line-wise operations `yy` / `dd` / `p` (copy/cut/paste lines)

**Request:** "There needs to be line-based operations like `yy` or `dd` and
`p` like in vim to either delete or copy and paste complete lines. Make a
suggestion which is cohesive with the current implementation."

**Constraint that shapes everything:** the editor body is
`bubbles/textarea` (`editor.go`, `e.ta`), which is **modeless** (always
insert-like) and exposes no vim modes. Real `yy`/`dd`/`p` are Normal-mode
commands; this editor has no Normal mode. So a literal vim binding doesn't
fit. Two cohesive options — RECOMMEND one, flag for user confirmation at
pick-up (per memory `todo_md_tracking`, nontrivial-UX decisions go to the
user):

- **(A, recommended) Ctrl-based line ops, no modes.** Bind `Ctrl+Y` = yank
  current line, `Ctrl+D` = delete (cut) current line, `Ctrl+P` = paste the
  line register below the cursor. Fits the existing "Ctrl+ for editor
  actions" convention (save is Ctrl+S; see memory `cli_standards` on
  Ctrl/Shift conventions) and needs no mode machinery. Downside: `Ctrl+D`
  traditionally = EOF/half-page; confirm it's unbound here first.
- **(B) A minimal `g`-less "line command" prefix.** Reserve a leader (e.g.
  a double-tap detector for `dd`/`yy`) inside `updateBody`. More vim-faithful
  but introduces timing/statefulness (partial-chord tracking) that the
  codebase doesn't otherwise have — heavier, more failure modes.

**Implementation sketch (for whichever binding wins):** operate on
`e.ta.Value()` split by `\n` at `e.ta.Line()` (the current line index, used
already in `handleEnter`, `editor.go:538`). A single in-`editPane` string
field `lineRegister` holds the yanked/cut line. Yank: copy the line.
Delete: remove the line from the value and `SetValue` (mind cursor
placement — clamp to the new line). Paste: insert `register + "\n"` after
the cursor line. Update `wordCount`/`dirty` after mutation. This is a
`Value()`/`SetValue()` string operation — it does NOT need the textarea
fork (item 8), unlike selection.

**Tension to flag (CLAUDE.md "not a text editor"):** same drift note as
item 9 — vim line ops expand the inline editor. Surface to user.

**Open questions:** (1) does the register survive across notes / editor
sessions, or reset per open? Recommend: per-editor-session, reset on close
(simplest). (2) Should `p` paste below (vim `p`) or at cursor? Recommend
below, matching vim. Decide with the user when picking the binding.

**Implementation notes:** not started. Verify: yank a line, paste it →
duplicated; delete a line → gone and Ctrl+Z-recoverable (route the mutation
through the same dirty/save/undo path as normal edits so undo works).
Headless tests per operation. Manual tmux pass.

---

## Tasks epic (items 15–18) — shared context

Items 15–18 together implement the task-management feature set the user
requested. They share **on-disk storage-format decisions** that must be
settled once, in item 15, before the view work in 16–17. Do 15 first.
The current task model is deliberately minimal (CLAUDE.md "Tasks":
standard Markdown `- [ ]`/`- [x]` only, no custom syntax) — these items
extend it, so the storage-format additions below are a conscious departure
from that line and should be run past the user (they also arguably belong
in `spec.html`, which today only specifies frontmatter, not inline task
syntax).

Current task plumbing to build on: `checkboxLineRe` (`viewer.go:26`,
`^(\s*[-*]\s+)\[([ xX])\](.*)$`), `toggleCheckboxLine` (`viewer.go:457`),
`checkboxLines` map + `checkboxRawLineAt` (`viewer.go:435`). Tasks live
inline in note bodies; there is no separate task store or index today
(the SQLite index in `internal/index/` does not index tasks yet —
CLAUDE.md notes "Tasks may be indexed by the system" as a maybe).

---

## 15. [ ] Task storage format: `-->` results + a completion date

**Request (two coupled additions):**
- "Tasks can have results, written like `-->` after the task. Results can be
  strings or links." → `- [x] Ship the thing --> shipped in v2` or
  `- [x] Read paper --> [[202606241530 Notes]]`.
- Finished tasks show "the date when it was toggled done last (overwritten
  by multiple state changes)" (see item 16 for the display). That date must
  be **persisted in the markdown line**, since nothing else survives a
  reload.

**Storage-format decisions — RECOMMENDED DEFAULTS, confirm with user:**
- **Result grammar:** ` --> ` (space-arrow-space) separates task text from
  result; everything after the first ` --> ` on the line is the result.
  A result that parses as `[[wikilink]]` is rendered/opened as a link
  (reuse the viewer's existing wikilink handling, `linkAtLine`
  `viewer.go`); otherwise it's plain text. Extend `checkboxLineRe` or add a
  second regex `resultRe` — do NOT break the existing capture groups that
  `toggleCheckboxLine` depends on.
- **Completion date:** adopt Obsidian-Tasks convention `✅ YYYY-MM-DD`
  appended to the line, e.g. `- [x] Task ✅ 2026-07-09 --> result`. On
  toggle-to-done, stamp/overwrite today's date; on toggle-to-undone, strip
  it. Store date-only (the request says "date," not time) — if the user
  wants time too, `✅ YYYY-MM-DDTHH:MM` is the extension point. Parsing must
  tolerate lines with the date, without it, with a result, with both, in
  either order — pin the canonical order as `text ✅ date --> result` and
  normalize on write.

**Where it changes:** `toggleCheckboxLine` (`viewer.go:457`) is the single
write path for done/undone — it must now also stamp/strip `✅ date`.
Add a parse helper (e.g. `parseTaskLine(raw) -> (marker, done, text,
date, result)`) used by both the viewer render and item 17's overview.
Keep `checkboxLineRe` working or migrate all its callers together (grep
`checkboxLineRe` in `internal/tui/` first).

**Backward-compat:** existing notes have plain `- [x] text` with no date /
result — every parser must treat those as valid (date="", result=""). Do
not rewrite untouched lines on load; only stamp the date on an actual
toggle, so opening an old note doesn't churn the whole file.

**Implementation notes:** not started. This is the foundation for 16 and
17 — get the parser + round-trip (parse→render→parse) tested at the vault
level before any UI. Verify: toggle done → line gains `✅ <today>`; toggle
again → date gone; add ` --> [[Note]]` by hand → parses as link.

---

## 16. [ ] Finished tasks sink to the bottom of their list + table-like styling

**Request:** "If a task is toggled finished it moves to the bottom of the
task list it's in — grouping is always unfinished on top, finished on
bottom. Finished tasks at the bottom get a different, more table-like
styling: the toggle box (so you can un-toggle an accidental done), the
date it was last toggled done, the task itself, and a result."

**THE key ambiguity — resolve before building. Physical vs. visual:**
"moves to the bottom" can mean (a) **rewrite the `.md`** so done lines
physically sink within their list block, or (b) **render** done tasks at
the bottom while the file's line order is untouched. **Recommend (b),
view-layer only**, and flag to the user. Reasons: (a) mutates the user's
document order on every toggle (surprising, destructive, and fights any
external editor / git diff); the request's own cues — "un-toggle if
accidental" and "table-like styling" — describe a **render treatment**, not
a file rewrite. Under (b), toggling only flips the box (+ stamps the date,
item 15); the reorder is purely how the viewer draws the list.

**Scope of "the task list it is in":** a "list" = a contiguous block of
task lines (consecutive `checkboxLineRe` matches, allowing nested
indentation). Reordering is within each such block independently — do not
move a done task across a non-task paragraph into another block.

**Rendering (view layer):** in the viewer's body build
(`processCheckboxesAndCode` / `buildBodyMd`, `viewer.go:114`/`:519`), when
emitting a task block, split into unfinished-then-finished and render the
finished group with the table-like style: `[x]` box (still toggleable —
keep it wired to `checkboxRawLineAt` so Space/Enter from items 12–13 still
un-toggle it), the `✅ date` (item 15), the task text, and the ` --> `
result (item 15; render wikilink results as links). Keep the rendered→raw
line mapping (`checkboxLines`, `viewer.go:435`) correct after the visual
reorder — this is the fiddly part: the map must point each rendered row at
its true raw body line so toggling still edits the right line.

**Interaction with item 12 (cursor-stays-put):** after a toggle causes a
visual reorder, "cursor stays where it is" is ambiguous. Recommend: cursor
holds the same **rendered row position** (screen-stable), not follow the
task as it moves. Settle this here and note it in item 12.

**Depends on:** item 15 (needs `✅ date` + `-->` parsing). Do 15 first.

**Implementation notes:** not started. Verify in tmux: a list with mixed
done/undone renders undone-on-top, done-at-bottom in table style; toggling
an undone task moves it down; un-toggling a done one moves it back up;
toggling never corrupts which raw line gets edited (test with duplicate
task texts). Headless test on the reorder + line-map integrity.

---

## 17. [ ] Task Overview: a new view collecting all tasks, grouped by source

**Request:** "Build a new view page collecting every task, grouped by
source (styled as headings). Hierarchy: Projects → all tasks in that
project categorized by file (Project = H1, files = H2). After that, tasks
not related to projects, grouped by file. Add it as a navigation-menu
entry AND a command — suggest a command."

**Suggested command: `:tasks`** (recommendation, not an open question —
the user asked for a suggestion). Register it in the `commands.go` dispatch
switch (`commands.go:45`, alongside `open`/`search`/etc.) as `cmdTasks`,
opening the new view in the active split. Add `tasks` to the palette's
command list / autosuggest (see memory `cli_standards` checklist for adding
a command — palette registration, help_view.go, README).

**New view — follow the existing pattern:** views are a `view` enum
(`model.go:20-27`: `viewList`, `viewProjectsOverview`, `viewProjectDetail`,
`viewSectionLanding`, `viewEdit`, `viewNote`, `viewHelp`). Add
`viewTasksOverview`. Model the rendering on `project_views.go`
(`viewProjectsOverview` is the closest analog — a full-pane list grouped
under headers) and wire its render in `model.go`'s view switch (the
`viewProjectsOverview` render + the `case viewProjectsOverview:` key
handler at `model.go:663-666` show the Esc/backspace-to-close idiom).

**Data assembly:** scan every note in the vault for task lines (reuse the
item-15 parser). Group: for notes with `State == StateProjects` and a
`Project`, bucket under Project (H1) → note/file (H2) → its tasks; all
other notes' tasks go in a trailing "Unassigned / by file" group keyed by
note. There is no task index today, so v1 can scan `m.vault` notes on view
open (fine for local vaults; note that indexing tasks in
`internal/index/` is a future optimization, not needed for v1 — CLAUDE.md
already hedges "may be indexed"). Sort projects by the same order the
sidebar uses (`Projects.ListActive()`), files by title.

**Interactivity (decide, recommend minimal):** v1 = read-only overview;
Enter on a task opens its source note (reuse `openOrCreateNote`).
Toggling tasks directly from the overview is a nice-to-have — defer unless
the user asks, to keep v1 simple. Note the decision.

**Nav entry:** add a sidebar row for "Tasks" that opens `viewTasksOverview`
(see `sidebar.go` `items()`, `sidebar.go:108` — mirror the virtual
`#templates` section row at `sidebar.go:131-134`, which is the existing
precedent for a non-note-state nav entry). Its visibility is controlled by
item 18.

**Depends on:** item 15 (task parser). Nav-visibility toggle is item 18.

**Implementation notes:** not started. Verify: create tasks across a
project note and a loose Inbox note, run `:tasks` and open via the nav
entry → both show the grouped hierarchy; Enter on a task opens its note.
Headless test on the grouping/assembly. Manual tmux pass. Update README +
help_view.go + All Commands table (memory `readme_updates`,
`cli_standards`).

---

## 18. [ ] Config: show/hide the Tasks quick-link and Templates in the nav

**Request:** "New config-menu entry to display the task quick-link in the
navigation menu or hide it — and for Templates too while you're at it."

**Where it changes:**
- **Config model:** add two `bool` fields to `AppConfig`
  (`appconfig.go:18`), e.g. `ShowTasksNav` and `ShowTemplatesNav` (default
  both `true` so current behavior is unchanged). Add defaults in
  `defaultConfig` (`appconfig.go:88`) and `fillConfigDefaults`
  (`appconfig.go:118`) — follow the exact pattern `LineNumbers` uses
  (it's the existing bool toggle). Bump `currentConfigVersion` per that
  file's versioning convention if fill-defaults needs it.
- **Config UI:** add two toggle rows to the General section of the config
  pane — extend the `cfgItem` enum (`config_pane.go:16`, currently
  `cfgItemLineNumbers`) and the General-section render/`changeValue` path
  (`config_pane.go:174`, mirror how `cfgItemLineNumbers` maps its bool at
  `config_pane.go:105-106`).
- **Sidebar honors the flags:** in `sidebar.go` `items()`
  (`sidebar.go:108`), gate the Templates section row (`sidebar.go:131-134`)
  on `ShowTemplatesNav`, and gate the new Tasks nav row (item 17) on
  `ShowTasksNav`. The sidebar needs access to the config — check how it's
  currently constructed (`newSidebar`, `sidebar.go:50`) and thread the two
  bools in (or pass `AppConfig`), matching however `LineNumbers` reaches
  the editor today (`model.go:544` passes `m.cfg.LineNumbers` explicitly —
  same style).

**Depends on:** item 17 for the Tasks row to exist (Templates half can
ship independently — it's just gating an existing row).

**Implementation notes:** not started. Verify: toggle each off in `:config`
→ the corresponding nav row disappears and stays hidden across restart
(config persists via `saveConfig`); toggle on → reappears. Headless test on
`items()` respecting both flags. Update README config section +
help_view.go.

---

## 19. [ ] GitHub Issues + GitLab Issues API — PLACEHOLDER MARKER ONLY

**Request (verbatim intent):** "Big new feature: add APIs for GitHub Issues
and GitLab Issues. This is just a marker for now, spec is not fully formed,
will be fleshed out later." **Do NOT build this.** This entry exists only
so the intent isn't lost.

**Hard blockers before any work (all violate current charter — needs
explicit user sign-off):**
- CLAUDE.md core principles: "Local-first, Markdown-only" and "**No AI
  integration**" — an external Issues API is the first network/remote
  integration in the project and departs from local-first. This is a
  charter-level change, not a feature; the user must confirm the direction
  and how issues map to the PARA/Markdown model (imported as notes? a new
  entity? read-only mirror vs. two-way sync?).
- Auth/secrets handling (tokens) has no home in the current config model
  and shouldn't be invented speculatively.

**When it's picked up:** the user will provide the real spec. At that point
turn this into its own set of numbered items. Until then: no code, no deps,
no config fields.

**Implementation notes:** not started; placeholder only, by explicit user
instruction.

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
