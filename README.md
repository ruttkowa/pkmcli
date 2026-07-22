# pkm

A terminal-based Personal Knowledge Management tool — navigate, organize, and search your Markdown notes without leaving the keyboard.

- **Local-first** — plain `.md` files, no lock-in
- **Keyboard-first** — everything reachable without a mouse
- **Not an editor** — writing delegates to your preferred external editor (nvim, vim, helix, …)... except for the built-in inline editor (`e`), which handles day-to-day edits without leaving the app
- **No AI, no sync, no cloud**

## Contents

- [Quick start](#quick-start)
- [Layout](#layout)
- [Reference: modes, hotkeys & commands](#reference-modes-hotkeys--commands) — the definitive list of every key and every `:command`
- [Knowledge model](#knowledge-model)
- [Projects workflow](#projects-workflow)
- [Templates](#templates)
- [Note format](#note-format)
- [Links](#links)
- [Vault structure](#vault-structure)
- [Configuration](#configuration)
- [File watcher](#file-watcher)
- [Running under tmux / zellij](#running-under-tmux--zellij)
- [Running tests](#running-tests)
- [Developer reference (CLI standards)](#developer-reference-cli-standards)

---

## Quick start

```sh
./install.sh
pkm [path/to/vault]
```

`install.sh` builds the binary and installs it to `~/.local/bin` (override with `PREFIX=/some/dir ./install.sh` or `./install.sh /some/dir`), adding it to your shell's `PATH` if it isn't already there. Run `./install.sh --help` for details.

Without the install script:

```sh
go build -o pkm ./cmd/pkm
./pkm [path/to/vault]
```

On first run the vault directory is created automatically. Omit the path to be prompted. Once it's open, press `?` for in-app help at any time — it mirrors the reference below.

---

## Layout

```
┌─ PKM  ›  Inbox ──────────────────────────────────────────────────────┐
│                                                                       │
│  Sidebar 25%          │  Main pane 75%                               │
│                       │                                               │
│  SECTIONS             │  Note viewer / list / project detail         │
│  ▼ Inbox (3)          │                                               │
│    · Docker Setup     │                                               │
│    · Meeting Notes    │                                               │
│  ▶ Projects (2)       │                                               │
│    ▼ Homelab (1)      │                                               │
│      · Docker Setup   │                                               │
│    ▶ Side Project     │                                               │
│  ▶ Areas              │                                               │
│  ▶ Research           │                                               │
│  ▶ Archive            │                                               │
│  ▶ #templates         │                                               │
│                       │                                               │
├───────────────────────┴───────────────────────────────────────────────┤
│ :  command   ?  help   SHIFT+ [N new] [A archive] …   CTRL+ [Q quit] │
└───────────────────────────────────────────────────────────────────────┘
```

`Tab` toggles focus between the two panes. The main pane shows one of several views depending on context (note list, note viewer, editor, project detail, help, config) — `:split` opens more of them side by side.

---

## Reference: modes, hotkeys & commands

This is the complete, ground-truth list of every keybinding and `:command` — everything below is current as of this README, not aspirational.

### How modes capture input

pkm doesn't have a named `Mode` enum, but input already branches by context. Five overlays capture **every** keystroke while active — global shortcuts (`N`/`O`/`A`/`M`/`T`/`P`/`?`/`:`/Tab/etc.) are blocked from leaking through, so typing normally inside them never gets hijacked:

| Overlay | Entered via | Exits via |
|---|---|---|
| Command Palette | `:` or the Palette key (default `Ctrl+Space`) | `Enter` (run) / `Esc` (cancel) |
| Editor | `e` on an open note | `Ctrl+S` (save) / `Esc` (discard) |
| Project Detail — bridge entry | `e` / `i` inside Project Detail | `Enter` (submit) / `Esc` (cancel) |
| Config overlay | `:config` | `Esc` (save & close) |
| Pane picker | the Pane picker key (default `Ctrl+P`) | `Enter` (confirm) / `Esc` (cancel) |

Everywhere else — Sidebar, Note List, Note Viewer, Help — has no free-text field to protect, so global shortcuts intentionally pass through.

Two rules apply everywhere, in every mode:

- **`Ctrl+C` is always rewritten to `Esc`** before any mode sees it — it cancels/closes, it never quits the app. The one exception: in the Note Viewer with an active text selection, `Ctrl+C` copies the selection instead (see [Text Selection](#text-selection) below).
- **Quitting only happens via** the Quit key (default `Ctrl+Q`), `Ctrl+D`, or `:quit`/`:exit`.

`Ctrl+Space`, `Ctrl+P`, `Ctrl+W`, `Ctrl+Q`, `Ctrl+Z`, `Ctrl+Y`, and `Ctrl+S` below are **defaults** — every one of them is remappable in [`:config` → Keybindings](#config-overlay) if it collides with your terminal multiplexer's own prefix (see [Running under tmux / zellij](#running-under-tmux--zellij)).

### Global

Active whenever no overlay above has captured input.

| Key | Action |
|---|---|
| `:` or **Palette** (`Ctrl+Space`) | Open the command palette |
| `?` | Toggle the Help view |
| `Tab` / `Shift+Tab` | Switch focus: Sidebar ↔ Main pane |
| **Pane picker** (`Ctrl+P`) | Open the pane picker |
| **Next pane** (`Ctrl+W`) | Cycle to the next split (only shown once you have >1) |
| **Undo** (`Ctrl+Z`) | Undo the last note save |
| **Redo** (`Ctrl+Y`) | Redo |
| **Quit** (`Ctrl+Q`), `Ctrl+D` | Save session and quit |
| `e` | Open the currently-viewed note in the editor |
| `N` | Palette prefilled `new ` |
| `O` / `S` | Palette prefilled `open ` |
| `A` | Palette prefilled `archive ` (+ current note title, if one is open) |
| `D` | Palette prefilled `delete ` (+ current note title, if one is open) |
| `M` | Palette prefilled `move ` (+ `<title> → `, if one is open) |
| `T` | Palette prefilled `insert ` (template insert) |
| `P` | Palette prefilled `add project ` |
| `I` | Open the **Import** popover directly (not via the palette) |

Mouse: left-click is supported throughout (sidebar items, note-list rows, `[[links]]` and checkboxes in the viewer); click-dragging over plain body text in the viewer selects and auto-copies text (see [Text Selection](#text-selection)). The scroll wheel scrolls whichever pane (sidebar, note list, viewer, help) is under the cursor, without changing focus.

### Sidebar

| Key | Action |
|---|---|
| `j` / `↓` | Cursor down |
| `k` / `↑` | Cursor up |
| `→` | Expand section / project folder |
| `←` | Collapse section / project folder |
| `Enter` | Open note, or toggle a project/section and show its detail/landing page |

Mouse: clicking a row's `▶`/`▼` glyph toggles expand/collapse only; clicking anywhere else on the row always shows that row's view (section landing, project detail, or note), leaving the expanded state untouched.

### Note List

The note-list view (search results, section browsing).

| Key | Action |
|---|---|
| `j` / `↓` | Cursor down |
| `k` / `↑` | Cursor up |
| `Enter` | Open the highlighted note |

### Note Viewer

| Key | Action |
|---|---|
| `j` | Scroll down |
| `k` | Scroll up |
| `↑` `↓` `←` `→` | Move the block cursor through the rendered text, character by character (see below) |
| `Enter` | Act on whatever the block cursor is over |
| `[` / `Alt+←` | Back in this pane's note history |
| `]` / `Alt+→` | Forward in pane history |
| `Esc` / `Backspace` | Back (pops history; returns to the note list once history is empty) |
| Click a `[[link]]` | Open it — creates the note first if it doesn't exist yet |
| Click a `- [ ]` checkbox | Toggle it and save |
| `←` on a heading | Collapse it |
| `→` on a heading | Expand it |
| Click a heading | Toggle it collapsed/expanded |

**Block cursor:** a solid (non-blinking) cursor that moves through the rendered note independently of `j`/`k` scrolling, auto-scrolling the viewer when it reaches the edge. When it sits on a link, checkbox, or fenced code block, that element is highlighted. `Enter` then:

- on a **link** — opens the linked note (creating it first if it doesn't exist)
- on a **checkbox** — toggles `- [ ]` ↔ `- [x]` and saves
- on a **code block** — copies its contents to the clipboard via OSC 52 (works over SSH and inside tmux/zellij)

`Space` also toggles a checkbox, as long as the cursor is anywhere on that task's line — not just on the `[ ]` itself. On a non-task line, `Space` does nothing (it doesn't open links or copy code, unlike `Enter`).

**Finished tasks sink to the bottom.** Within each contiguous block of task lines, unfinished tasks are always shown first and finished tasks last (each group keeping its original relative order) — toggling a task done moves it to the bottom of its block on screen, and un-toggling moves it back. This is display-only: the `.md` file's line order never changes, only the rendered position. Finished tasks also render with a muted, secondary style so the unfinished ones stand out. The checkbox stays toggleable in place either way (un-toggle an accidental done the same way you toggled it).

The bottom row of the pane is a fixed footer showing `Last saved: HH:MM:SS` (from the note's `updated` frontmatter field) alongside the scroll percentage — not shown in the global hotkey bar.

**Cursor position carries across View ↔ Edit.** Pressing `e` opens the editor at the raw line the block cursor was on (not always the top); saving or Esc-closing the editor returns the block cursor to that same line (not the top). This is line-level and best-effort, not character-exact: glamour reflows markdown into a different line layout than the raw file (word-wrap, list indentation, link aliasing), so there's no exact inverse for every rendered position — landing on the correct *line* is the guarantee, not a specific column.

**Foldable headings.** Every heading shows a `▼`/`▶` glyph. Collapsing one (`←`, or a click, on the block cursor's heading line) hides everything until the next heading of the same or higher level — nested sub-headings inside a collapsed section stay hidden too, regardless of their own fold state. `→` expands it again, or click it a second time. Fold state is view-only: it's per-note, resets when you switch notes, and never touches the file on disk.

#### Text Selection

View mode only — the editor has its own separate Ctrl+L line operations (see below) and no selection of its own yet.

| Key / Mouse | Action |
|---|---|
| `Shift+↑↓←→` | Extend the selection from the block cursor's position (the anchor is set the first time you shift-move) |
| **Ctrl+A** | Select the entire note |
| **Ctrl+C** | Copy the selection to the clipboard via OSC 52 |
| `Esc` | Clear the selection (a second `Esc` then does its usual "back") |
| Any plain (non-shift) cursor movement | Clears the selection |
| Click-drag over body text | Selects from press to release; releasing **copies automatically**, no `Ctrl+C` needed |

The status bar reports how many characters were copied (`copied N characters`) — OSC 52 has a per-terminal size limit, so this makes a silently-truncated large copy (e.g. `Ctrl+A` on a long note) visible instead of failing invisibly.

The copied text is always the note's raw Markdown source, not the rendered/ANSI output and not resolved `[[link|alias]]` text. Because that raw text is recovered via the same rendered→raw line mapping the block cursor uses to survive View↔Edit round-trips (best-effort, line-level — see "Cursor position carries across View ↔ Edit" above), a selection copies the **full raw lines** its start and end touch, not an exact character span within them — the highlight you see follows the same whole-line granularity.

There is no `Ctrl+V` paste in the viewer: view mode has no editable buffer to paste into. Pasting is out of scope until the app has a proper editable-selection buffer.

### Editor

Opened with `e`. Captures all input — the only thing that reaches it from outside is the **Palette** key, which opens the palette as a live overlay on top of your still-open draft (see [Templates](#templates) for what `:insert` does there).

| Key | Action |
|---|---|
| **Save** (`Ctrl+S`) | Commit the draft and return to the viewer |
| `Esc` | Save the draft (if changed) and return to the viewer — same as **Save**, just via a different key. `Ctrl+Z` undoes it afterward like any other save. |
| `Tab` / `Shift+Tab` | Cycle focus: State → Tags → Project → Body |
| `←`/`h`, `→`/`l`/`Enter` | Cycle value (State field only) |

In the body field:

| Key | Action |
|---|---|
| `[`, `(`, `` ` `` | Auto-pairs `[]`, `()`, `` `` `` with the cursor placed between |
| `]`, `)`, `` ` `` | Skips over the matching closer if the cursor is right before one |
| `Enter` | Continues `- `, `- [ ] `/`- [x] `, `* `, and numbered (`1.` → `2.`) lists. Pressing it again on a marker with no text after it (i.e. right after the previous Enter auto-added one) clears that marker instead of adding another — the way to break out of a list. (This app's Bubble Tea/terminal stack can't distinguish `Shift+Enter` from plain `Enter`, so this empty-marker break-out stands in for it.) |
| `Ctrl+L` | Line-op leader (see below) |

**Line operations.** `Ctrl+L` is a leader — the very next key runs one operation, and anything else (including `Esc`) cancels the chord without typing that key into the note:

| Chord | Action |
|---|---|
| `Ctrl+L` `y` | Yank (copy) the current line |
| `Ctrl+L` `d` | Delete (cut) the current line |
| `Ctrl+L` `p` | Paste the last yanked/deleted line below the cursor |

The register holds one line for the current editor session only (reset when the editor closes — it doesn't carry across notes). Pasting with an empty register is a no-op. A deleted line is `Ctrl+Z`-recoverable the same way any other save is, since the mutation goes through the normal draft body rather than a side channel. Direct `Ctrl+Y`/`Ctrl+D`/`Ctrl+P` bindings were considered and rejected — all three are already taken (Redo, Quit-alias, Pane Picker/cursor-up).

Typing inside an unclosed `[[fragment]]` opens a link-autosuggest dropdown:

| Key | Action |
|---|---|
| `↑`/`↓` or `Ctrl+P`/`Ctrl+N` | Move selection |
| `Tab` / `Enter` | Accept the highlighted suggestion |
| `Esc` | Dismiss suggestions only — the editor itself stays open |

The **Project** field autosuggests from active projects the same way (case-insensitive prefix match, same `↑`/`↓`/`Tab`/`Enter`/`Esc` keys). Saving with a project name typed in creates the project if it's new (subject to the max-4-active-projects limit), forces the note into `projects` state, and reveals it in the sidebar's project tree — the same thing `:add project`/`P` does. Clearing the field and saving detaches the note and returns it to Inbox.

The footer row shows word/line counts plus `Last saved: HH:MM:SS`; while the draft differs from the saved note (body, tags, project, or state), an `● Unsaved changes` marker appears next to it. This footer — not the global hotkey bar — is where save state lives.

### Project Detail

Shows a project's attached notes, its history, and — below that — a **Tasks** section: a fully interactive, filtered version of the Task Overview scoped to just this project's notes, grouped by note the same way. Toggling a task here uses the same ✅-date-stamping rules as everywhere else, and edits the underlying note directly.

| Key | Action |
|---|---|
| `j` / `↓`, `k` / `↑` | Move the task cursor (skips note-header rows); falls back to scrolling if the project has no tasks |
| `Space` | Toggle the checkbox under the cursor |
| `Enter` | Open the source note of the task under the cursor |
| `e` / `i` | Start a Hemingway-bridge journal entry |

A project with no tasks shows a plain "(no tasks)" line instead of the section.

Composing a bridge entry (captures all input until submitted or cancelled — while this has focus, the task-list keys above are inactive):

| Key | Action |
|---|---|
| `Enter` | Submit (ignored if the entry is blank) |
| `Esc` | Cancel, discard the draft entry |
| `Backspace` / `Ctrl+H` | Delete last character |
| `Ctrl+U` | Clear the line |

### Task Overview

Opened with `:tasks` or the sidebar's **Tasks** row (below `#templates`; shown by default — see [Configuration](#configuration) to hide it). Scans every note in the vault for checkbox lines and groups them: each active project with task-bearing notes gets a heading, its notes (sorted by title) each get a sub-heading followed by their tasks in file order; a trailing **Unassigned** group covers every other task-bearing note the same way. Read-only in v1 — toggling a task's checkbox from here isn't supported yet, only opening its source note.

| Key | Action |
|---|---|
| `j` / `↓`, `k` / `↑` | Move the row cursor |
| `g` / `G` | Jump to the first / last row |
| `Enter` | Open the source note of the task under the cursor (no-op on a heading row) |
| `Esc` / `Backspace` | Close and return to the note list |

The overview is assembled fresh each time it's opened (there's no persistent task index yet), so it always reflects the vault as it is right now.

### Trash

`:delete` doesn't remove a note immediately — it moves the file into `<vault>/.pkm/trash/` and records it in `.pkm/trash.json`, recoverable for a configurable retention window (default 30 days, see [Configuration](#configuration)). `Ctrl+Z` right after deleting still works exactly as before, as the immediate safety net; trash is the durable one, for whenever you notice later. Past its retention window, a trashed note is permanently purged the next time pkm starts (no background timer).

Opened with `:trash`. Each row shows the note's title, how long ago it was deleted, and how many days are left before it's purged.

| Key | Action |
|---|---|
| `j` / `↓`, `k` / `↑` | Move the row cursor |
| `g` / `G` | Jump to the first / last row |
| `Enter` | Restore the note under the cursor to its original location (falls back to Inbox if its project no longer exists) |
| `d` | Permanently delete the note under the cursor — press again to confirm (footer line, not a popup); any other key cancels the confirm. Irreversible: no undo record, since the note already had its `Ctrl+Z` chance. |
| `Esc` / `Backspace` | Close and return to the note list |

Like the Task Overview, the list is read fresh from `.pkm/trash.json` each time it's opened.

### Config Overlay

Opened with `:config`. `Tab` / `Shift+Tab` cycle its three tabs; `Esc` saves everything and closes, from any tab (while capturing a keybind or editing a variable, `Esc` cancels just that instead — see below).

**General** — `↑`/`↓` selects a setting, `←`/`→` cycles its value.

**Keybindings** — `↑`/`↓` selects an action, `Enter` starts capture ("press ctrl/alt + a key…"), `d` resets that action to its default. Only `Ctrl`/`Alt` chords are accepted while capturing, so you can't accidentally shadow a plain letter used elsewhere.

**Variables** — `↑`/`↓` selects a variable (or the **+ Add variable** row), `Enter` adds a new one or edits an existing value, `d` deletes.

### Import Popover

Opened with `I` or `:import [path]`. Imports an external markdown file into the vault as a new note, renamed to the `<ID> <Title>.md` convention (title taken from the source filename) with fresh `id`/`created`/`updated`/`state` — any existing `tags:` in the source file's frontmatter are preserved.

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Move between Path → Mode → Destination → Import |
| (Path field) typing | Live directory-listing suggestions (dirs and `.md` files, filtered as you type) |
| (Path field) `↑`/`↓` | Select a suggestion |
| (Path field) `Enter` | Accept the highlighted suggestion |
| (Mode field) `Space` | Toggle Move ↔ Copy — **default: Move** (source file is removed after import) |
| (Destination field) `←`/`→` or `Enter` | Cycle the destination state (default: Inbox) |
| (Import field) `Enter` | Run the import |
| `Esc` | Cancel — nothing on disk changes |

On success the popover closes and the imported note opens in the active pane. On failure (e.g. bad path) an error shows inside the popover and it stays open so you can correct it.

### Export Popover

Opened with `:export [path]`. Writes the currently open note's raw markdown (frontmatter included, byte-identical to the vault file — round-trips back in via `:import`) to a path outside the vault. Always a copy: the vault note is never modified, moved, or removed.

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Move between Path → Export |
| (Path field) prefill | Defaults to the open note's own `<ID> <Title>.md` filename — `Enter` alone exports to the current working directory under that name |
| (Path field) typing | Live directory-listing suggestions, same as Import |
| (Path field) `Enter` | Accept the highlighted suggestion |
| (Export field) `Enter` | Run the export — if the target already exists, one more `Enter` confirms the overwrite |
| `Esc` | Cancel — nothing on disk changes |

A bare directory path (e.g. `~/Documents/`) exports into it under the note's own filename. A nonexistent target directory is reported as an error rather than created implicitly. `:export` with no note open shows an error instead of opening an empty prompt.

### Pane Picker

Opened with the **Pane picker** key (`Ctrl+P` by default).

| Key | Action |
|---|---|
| `←`/`h`, `→`/`l` | Move selection (index 0 = Sidebar, 1..n = each split) |
| `Enter` or the Pane picker key again | Confirm |
| `Esc` | Cancel |

### Help View

Opened with `?`.

| Key | Action |
|---|---|
| `j` / `↓`, `k` / `↑` | Scroll |
| `g` / `G` | Jump to top / bottom |
| `Esc` / `Backspace` / `?` | Close |

### Command Palette

Opened with `:` or the **Palette** key.

| Key | Action |
|---|---|
| `↑`/`↓` or `Ctrl+P`/`Ctrl+N` | Navigate suggestions |
| `Tab` / `→` | Complete the highlighted suggestion |
| `Enter` | Run whatever text is currently typed (not necessarily the highlighted row) |
| `Esc` | Cancel |
| `Backspace` / `Ctrl+H` | Delete last character |
| `Ctrl+U` | Clear the line |

The verb list reorders a little depending on where you opened it from — e.g. opening it from the editor puts `:insert` first, since that's what you're almost always about to do mid-edit.

### All commands

| Command | What it does | Shift shortcut |
|---|---|---|
| `:new "Title"` | Create a note in Inbox | `N` |
| `:new project <name>` | Create a new project — does **not** assign the open note (max 4 active) | — |
| `:add project <name>` | Assign the open note to a project, creating it if it doesn't exist yet; moves the note to Projects | `P` |
| `:new template "Title"` | Create a new note pre-tagged `template` | — |
| `:insert <name>` | Insert a template into the open note | `T` |
| `:insert var <name>` | Insert a variable's value at the cursor — edit mode only | — |
| `:open <query>` | Open by title; else full-text search; `#tag` filters by tag | `O` / `S` |
| `:search <query>` | Fuzzy search titles and content, with a live-ranked dropdown as you type | — |
| `:move <note> → <state>` | Move a note to a state (`->` also accepted) | `M` |
| `:archive <note>` | Shortcut for `:move <note> → archive` | `A` |
| `:delete <note>` | Move a note to trash (`Ctrl+Z` undoes it immediately — see note below) | `D` |
| `:import [path]` | Open the import popover (path pre-filled if given) — see note below | `I` (opens directly, no palette) |
| `:export [path]` | Open the export popover, prefilled with the open note's filename — see note below | — |
| `:tasks` | Show every task in the vault, grouped by project then file | — |
| `:trash` | List deleted notes; `Enter` restores, `d` permanently deletes — see [Trash](#trash) | — |
| `:split [note]` | Open a new side-by-side pane, optionally pre-loaded | — |
| `:close` | Close the focused pane (blocked if it's the last one) | — |
| `:theme <name>` | `nord` · `solarized` · `dracula` · `gruvbox` · `tokyonight` | — |
| `:config` | Open the settings overlay | — |
| `:config theme <name>` | Set the theme directly, without opening the overlay | — |
| `:config export [path]` | Write config to a file (default `.pkm/config-export.yaml`) | — |
| `:config import [path]` | Load config from a file | — |
| `:help` | Open the help view | `?` |
| `:quit` / `:exit` | Save session and quit | — |

> [!NOTE]
> `:new project` and `:add project` are **not** aliases of each other, despite the similar names: `new project` only creates the project; `add project` is what actually attaches your currently-open note to one.

**`:open` resolution:** exact title match → opens directly · no match → full-text search → shows a result list · `#tag` prefix → tag filter.

**`:search` resolution:** the dropdown ranks fuzzy title matches first, then plain content matches, live as you type. Arrow to a hit and press `Enter` to open it directly; press `Enter` without arrowing to open every hit as a list instead. `Esc` out of a note opened either way returns to that list — not a fresh search.

**`:delete` moves the note to trash**, not a permanent removal — see [Trash](#trash). It's *also* pushed onto the same undo stack as edits, so `Ctrl+Z` immediately after recreates the file without even going through the trash/recovery flow. There's no confirmation prompt, so double-check the note name in the palette before pressing Enter; `:trash` and its retention window are the safety net for everything after that immediate undo window closes.

**`:import`** reads an external `.md` file and adds it as a new note — see [Import Popover](#import-popover) for the full field-by-field walkthrough. Default is **Move** (the source file is deleted after a successful import); toggle to **Copy** with `Space` on the Mode field to leave the source in place.

**`:export`** writes the open note's raw markdown to a path outside the vault — see [Export Popover](#export-popover). Always a copy; the vault note is never touched.

**Valid states for `:move`:** `inbox` · `projects` · `areas` · `research` · `archive`

---

## Knowledge model

Notes flow through states. All notes start in **Inbox**.

```
                    ┌─────────────┐
                    │    Inbox    │  ← all new notes land here
                    └──────┬──────┘
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
    ┌──────────┐     ┌──────────┐     ┌──────────────┐
    │ Projects │     │  Areas   │     │  Research    │
    │ (max 4)  │     │          │     │              │
    └──────────┘     └──────────┘     └──────────────┘
          │                 │                 │
          └─────────────────┴─────────────────┘
                            │
                            ▼
                    ┌──────────────┐
                    │   Archive    │
                    └──────────────┘
```

---

## Projects workflow

Projects are named collections of notes with a **4-project limit** (a GTD/PARA constraint to keep focus).

### Assigning a note to a project

```
Step 1 — open a note (any state, usually Inbox)
Step 2 — press P  or  type :add project <name>
          → note moves to Projects automatically
          → project is created if it doesn't exist
          → autosuggest shows existing project names
Step 3 — the note appears under its project in the sidebar
```

### Sidebar layout for Projects

```
▼ Projects
  ▶ Homelab (2)       ← collapsed project folder
  ▼ Side Project (1)  ← expanded project folder
    · Landing Page    ← click to open in main pane
```

### Project detail page

Select a project folder header to open its detail page (see [Project Detail](#project-detail) above for keys):

- List of attached notes (clickable)
- History log — every attach/detach event with timestamp
- **Hemingway bridge** — press `e` to add a timestamped journal entry
- **Tasks** — an interactive, project-filtered task list (move, toggle, jump to source note)

### Moving a note out of a project

```
:move <note> → inbox
```

The detach event is recorded in the project history.

---

## Templates

There are two kinds of templates.

### Creation templates (auto-applied on `:new`)

Place a `.md` file in `vault/templates/`. It is applied when you create a note.

| Variable | Replaced with |
|----------|--------------|
| `{{id}}` | Generated note ID |
| `{{title}}` | Note title |
| `{{created}}` | Creation timestamp |
| `{{updated}}` | Last updated timestamp |

### Insert templates (appended on demand)

Any note tagged `template` in its frontmatter is an insert template.
Create one with `:new template "Title"` (auto-tags it).

```
Step 1 — press T  or  type :insert <name>
Step 2 — autosuggest filters template notes as you type
Step 3 — template body is appended to the open note
```

Insert templates appear in **#templates** in the sidebar.

`{{id}}`, `{{title}}`, `{{created}}`, `{{updated}}`, and any variable defined
in `:config` → Variables are all substituted when the template is inserted.
Insert a single variable on its own with `:insert var <name>` — see
[All commands](#all-commands).

---

## Note format

**Filename:** `<ID> <Title>.md` — e.g., `202606241530 Docker.md`

**Frontmatter:**
```yaml
---
id: 202606241530
title: Docker
created: 2026-06-24T15:30:00
updated: 2026-06-24T15:45:00
state: inbox
project:
tags:
  - linux
  - containers
---

Note body here. [[Wikilinks]] are supported.
```

**Tasks:** standard Markdown checkboxes, `- [ ]` / `- [x]`. Plain checkboxes with no metadata are always valid — nothing is rewritten until the box is actually toggled. A checkbox line may optionally carry a completion date and a result, in canonical order `text ✅ YYYY-MM-DD --> result`:

```
- [x] Ship the thing ✅ 2026-07-10 --> shipped in v2
- [x] Read paper ✅ 2026-07-10 --> [[202606241530 Notes]]
```

The date is stamped when a task is toggled done and stripped when toggled back to undone (overwritten by repeated toggles, not accumulated). A result is free text after `-->`; if it parses as a `[[wikilink]]` it renders and opens as a link like any other wikilink. See [Task Overview](#task-overview) for the `:tasks` view that collects every task in the vault.

---

## Links

```
[[Docker]]                   → link to a note
[[Docker|Container Runtime]] → link with display alias
```

- Links to notes that don't exist yet are valid — the note is created when you open the link.
- In read mode, `[[Docker|Container Runtime]]` renders as `Container Runtime`.
- Working links are highlighted; broken links are shown as strikethrough.
- Click a link in the viewer to navigate to it.

---

## Vault structure

```
vault/
├── notes/         ← all notes as .md files
├── templates/     ← creation templates (optional)
└── .pkm/
    ├── config.yaml
    ├── projects.yaml
    ├── index.db     ← SQLite search index
    ├── trash.json   ← sidecar index for :delete'd notes (see Trash)
    └── trash/       ← the deleted notes' .md files, unmodified
```

If you keep your vault in git, add `.pkm/trash/` and `trash.json` to its `.gitignore` — deleted notes and their metadata aren't meant to be tracked history.

No content-based folder hierarchy — organization is through metadata, tags, and states.

---

## Configuration

Open the config menu with `:config` (see [Config Overlay](#config-overlay) for the exact keys). It has three tabs — cycle with `Tab` / `Shift+Tab` — and `Esc` saves and closes from any of them.

### General

| Setting | Options | Default |
|---------|---------|---------|
| Theme | Nord, Solarized Dark, Dracula, Gruvbox, Tokyo Night | Nord |
| Sidebar width | 20%, 25%, 33% | 25% |
| Restore session | on, off | on |
| Line numbers | on, off | on |
| Show Tasks nav | on, off | on |
| Show Templates nav | on, off | on |
| Trash retention | 7, 14, 30, 60, 90 days | 30 days |

Session state (last open note, active section) is saved on quit and restored on next launch when "Restore session" is on.

"Trash retention" controls how long a `:delete`d note stays recoverable in `:trash` (see [Trash](#trash)) before being purged on a future startup.

"Show Tasks nav" and "Show Templates nav" control whether the sidebar's **Tasks** row and `#templates` section are shown at all — independent of each other, and applied live the moment you toggle them (no need to close the overlay first).

### Keybindings

Remaps the global chords most likely to collide with a terminal multiplexer's own prefix (see [Running under tmux / zellij](#running-under-tmux--zellij)):

| Action | Default |
|--------|---------|
| Command palette | `Ctrl+Space` |
| Pane picker | `Ctrl+P` |
| Next pane | `Ctrl+W` |
| Quit | `Ctrl+Q` |
| Undo | `Ctrl+Z` |
| Redo | `Ctrl+Y` |
| Save (editor) | `Ctrl+S` |

Mode-internal keys (arrows, `hjkl`, etc.) aren't remappable — only these seven global chords are.

### Variables

Simple key-value pairs used by `:insert var <name>` and by `{{name}}` substitution in `:insert <template>`.

### Export / import

```
:config export [path]   # default: .pkm/config-export.yaml
:config import [path]
```

Paths are relative to the vault root unless absolute. Importing a config written by a different version never crashes: unknown fields are ignored and any field missing from the file falls back to its default.

---

## File watcher

Changes to files in `vault/notes/` are picked up automatically. Edits made in an external editor appear in the viewer without restarting.

---

## Running under tmux / zellij

pkm hardcodes a handful of global chords, its own split-pane system, and mouse support — all of which can interact with a terminal multiplexer running underneath it.

### Keybinding collisions

`Ctrl+P` is zellij's default "pane mode" chord, and a `Ctrl+Space` tmux prefix remap (common) would swallow pkm's palette shortcut before pkm ever sees it. Remap the affected action in [`:config` → Keybindings](#configuration) rather than fighting your multiplexer's config.

### Mouse events

pkm enables mouse reporting, but **tmux does not forward mouse events to the program running inside it by default** — clicks and scroll go to tmux's own pane-select/copy-mode instead, and pkm's mouse support will silently appear broken. Add to `~/.tmux.conf`:

```
set -g mouse on
```

zellij forwards mouse events by default; no configuration needed.

### True color

tmux reports `TERM=tmux-256color` / `screen-256color`, which can make some terminal apps under-detect true-color support. If pkm's theme colors look off under tmux, add to `~/.tmux.conf`:

```
set -ga terminal-overrides ",*256col*:Tc"
```

### Nested panes

pkm has its own split-pane system (`:split`, and the Next pane / Pane picker keys). Running it inside an already-split multiplexer pane works, but nesting splits from both layers at once produces two layers of borders with different meanings. Simplest is to give pkm one multiplexer pane and let it own its own splits.

---

## Running tests

```sh
go test ./...
```

---

## Developer reference (CLI standards)

### DSL grammar

```
:<verb> [arg1] [→ arg2]
:<compound verb> [arg1]
```

- **Verb** — one or two lowercase words after `:` (e.g. `new`, `move`, `add project`, `new template`, `insert var`)
- Compound verbs use **longest-prefix matching** — `insert var ` wins over `insert ` when both match
- Multi-slot commands use `→` (or `->`) as a separator
- Quoting optional for free-text titles; quotes stripped before processing

### Autosuggest rules by slot kind

| Argument type | Match rule | Source |
|---------------|-----------|--------|
| Note / template title | Substring, case-insensitive | All vault notes, sorted by last updated |
| Search query | Fuzzy subsequence match on title, ranked above plain substring matches on content | All vault notes |
| State | Prefix, case-insensitive | Fixed list |
| Theme | Prefix, case-insensitive | Fixed list |
| Project name | Prefix, case-insensitive | Active projects in `projects.yaml` |
| Variable name | Prefix, case-insensitive | `:config` → Variables |

See [All commands](#all-commands) above for the full command-to-slot mapping — this table isn't duplicated here to avoid the two drifting apart.

### Verb ranking by context

`verbSuggestions()` in `palette.go` shows commands in `allCommands` declaration order by default. A palette opened via `newPalette(...).withContext(ctx)` reorders matches using a fixed priority table (`verbPriority`) keyed by `verbContext` — a hand-authored bias, not a learned/frequency ranking. Current contexts: `ctxNoteOpen` (bare `:` while a note is open) and `ctxEditing` (Palette key from inside the editor, where `:insert` ranks first). Add a new context by adding a `verbContext` constant, an entry in `verbPriority`, and passing `.withContext(...)` from the relevant call site in `model.go`.

### Adding a new command — checklist

1. Add a `cmdDef` to `allCommands` in `palette.go`
   - Compound verbs work automatically via longest-prefix match in `currentCmdDef()`
2. Add routing to `handleCommand` in `commands.go`
   - Compound commands: add `parts[0]+" "+parts[1]` check before the single-word switch
3. If a new argument type is needed: add `slotKind`, update `contextSuggestions()` and `tabComplete()` in `palette.go`
4. Add a `Shift+Key` handler in `model.go` if a shortcut is needed
5. If it's a new global chord prone to multiplexer collisions: add it to `Keymap` in `appconfig.go` instead of hardcoding the key string, and add a row to `keymapLabels`
6. Update `help_view.go`, the tooltip bar, and this README's [All commands](#all-commands) / [Global](#global) tables
