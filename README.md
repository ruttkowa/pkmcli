# pkm

A terminal-based Personal Knowledge Management tool — navigate, organize, and search your Markdown notes without leaving the keyboard.

- **Local-first** — plain `.md` files, no lock-in
- **Keyboard-first** — everything reachable without a mouse
- **Not an editor** — writing delegates to your preferred external editor (nvim, vim, helix, …)
- **No AI, no sync, no cloud**

---

## Quick start

```sh
go build -o pkm ./cmd/pkm
./pkm [path/to/vault]
```

On first run the vault directory is created automatically. Omit the path to be prompted.

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

---

## Navigation

### Focus & panes

| Key | Action |
|-----|--------|
| `Tab` | Switch focus: Sidebar ↔ Main pane |
| `Ctrl+W` | Cycle to next split pane |
| `Ctrl+P` | Open pane picker |

### Sidebar movement

| Key | Action |
|-----|--------|
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `Enter` | Open note / toggle section |
| `→` | Expand section or project folder |
| `←` | Collapse section or project folder |

### In the main pane (viewer)

| Key | Action |
|-----|--------|
| `j` / `↓` | Scroll down |
| `k` / `↑` | Scroll up |
| `[` / `Alt+←` | Navigate back in pane history |
| `]` / `Alt+→` | Navigate forward |
| `Esc` / `Backspace` | Go back to the note list |

---

## Commands

Press `:` to open the command palette. A fuzzy dropdown appears as you type.

| Control | Action |
|---------|--------|
| `↑` / `↓` | Navigate suggestions |
| `Tab` | Complete highlighted item |
| `Enter` | Run the command |
| `Esc` | Cancel |

### All commands

| Command | What it does |
|---------|-------------|
| `:new "Title"` | Create a note in Inbox |
| `:new project <name>` | Assign the open note to a project (moves to Projects) |
| `:add project <name>` | Alias for `:new project` |
| `:new template "Title"` | Create a new template note (auto-tagged) |
| `:insert <name>` | Insert a template into the open note |
| `:open <query>` | Open a note by title, content search, or `#tag` filter |
| `:move <note> → <state>` | Move a note to a state |
| `:archive <note>` | Move a note to Archive |
| `:split [note]` | Open a side-by-side pane |
| `:close` | Close the focused pane |
| `:config` | Open settings menu |
| `:help` | Open the help view |
| `:quit` / `:exit` | Save session and quit |

**`:open` resolution:** exact title match → opens directly · no match → full-text search → shows list · `#tag` prefix → tag filter

**Valid states for `:move`:**

```
inbox   projects   areas   research   archive
```

---

## Shortcuts

### Shift shortcuts — open palette pre-filled

| Key | Pre-fills | What it does |
|-----|-----------|-------------|
| `N` | `:new ` | Create a note |
| `O` or `S` | `:open ` | Open / search |
| `A` | `:archive ` | Archive a note |
| `M` | `:move ` | Move to a state |
| `T` | `:insert ` | Insert a template |
| `P` | `:add project ` | Assign to a project |

### Other keys

| Key | Action |
|-----|--------|
| `?` | Open / close help view |
| `e` | Open current note in the editor |
| `Ctrl+C` | Cancel / close (same as Esc) |
| `Ctrl+Z` | Undo last note save |
| `Ctrl+Y` | Redo |
| `Ctrl+Q` / `Ctrl+D` | Save session and quit |

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

| Key / Action | Effect |
|-------------|--------|
| `Enter` on project folder | Toggle expand/collapse + open detail |
| `→` on collapsed folder | Expand + load notes |
| `←` on expanded folder | Collapse |
| Click note under project | Open in main pane |

### Project detail page

Select a project folder header to open its detail page:

- List of attached notes (clickable)
- History log — every attach/detach event with timestamp
- **Hemingway bridge** — press `e` to add a timestamped journal entry

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

## Editor

Press `e` to open the current note in the built-in inline editor.

| Key | Action |
|-----|--------|
| `Tab` | Cycle between fields (title, tags, body) |
| `Ctrl+S` | Save |
| `Esc` | Cancel (discard changes) |

---

## Split panes

Open notes side by side:

```
:split [note]
```

| Key | Action |
|-----|--------|
| `Ctrl+W` | Cycle focus between panes |
| `Ctrl+P` | Open pane picker |
| `:close` | Close the focused pane |

Each pane has its own navigation history (`[` / `]` to go back/forward).

---

## Vault structure

```
vault/
├── notes/         ← all notes as .md files
├── templates/     ← creation templates (optional)
└── .pkm/
    ├── config.yaml
    ├── projects.yaml
    └── index.db   ← SQLite search index
```

No content-based folder hierarchy — organization is through metadata, tags, and states.

---

## Configuration

Open the config menu with `Ctrl+C` or `:config`. Navigate with `↑↓`, change values with `←→`, press `Esc` to save.

| Setting | Options | Default |
|---------|---------|---------|
| Theme | Nord, Solarized Dark, Dracula, Gruvbox, Tokyo Night | Nord |
| Sidebar width | 20%, 25%, 33% | 25% |
| Restore session | on, off | on |
| Line numbers | on, off | on |

Session state (last open note, active section) is saved on quit and restored on next launch when "Restore session" is on.

---

## File watcher

Changes to files in `vault/notes/` are picked up automatically. Edits made in an external editor appear in the viewer without restarting.

---

## Running tests

```sh
go test ./...
```

---

## Developer reference — CLI Standards

### DSL grammar

```
:<verb> [arg1] [→ arg2]
:<compound verb> [arg1]
```

- **Verb** — one or two lowercase words after `:` (e.g. `new`, `move`, `add project`, `new template`)
- Compound verbs use **longest-prefix matching** — `new project ` wins over `new ` when both match
- Multi-slot commands use `→` (or `->`) as a separator
- Quoting optional for free-text titles; quotes stripped before processing

### Autosuggest rules by slot kind

| Argument type | Match rule | Source |
|---------------|-----------|--------|
| Note / template title | Substring, case-insensitive | All vault notes, sorted by last updated |
| State | Prefix, case-insensitive | Fixed list |
| Theme | Prefix, case-insensitive | Fixed list |
| Project name | Prefix, case-insensitive | Active projects in `projects.yaml` |

### All commands at a glance

| Command | Alias | Slot | Shift key |
|---------|-------|------|-----------|
| `new "Title"` | — | title | `N` |
| `new project <name>` | — | project | — |
| `add project <name>` | — | project | `P` |
| `new template "Title"` | — | title | — |
| `insert <name>` | — | template | `T` |
| `open <query>` | — | note | `O` / `S` |
| `move <note> → <state>` | — | note + state | `M` |
| `archive <note>` | — | note | `A` |
| `split [note]` | — | note | — |
| `close` | — | — | — |
| `theme <name>` | — | theme | — |
| `config` | — | — | `Ctrl+C` (direct) |
| `help` | — | — | `?` |
| `quit` | `exit` | — | `Ctrl+Q` / `Ctrl+D` |

### Adding a new command — checklist

1. Add a `cmdDef` to `allCommands` in `palette.go`
   - Compound verbs work automatically via longest-prefix match in `currentCmdDef()`
2. Add routing to `handleCommand` in `commands.go`
   - Compound commands: add `parts[0]+" "+parts[1]` check before the single-word switch
3. If a new argument type is needed: add `slotKind`, update `contextSuggestions()` and `tabComplete()` in `palette.go`
4. Add `Shift+Key` handler in `model.go` if a shortcut is needed
5. Update `help_view.go`, tooltip bar, and this README
