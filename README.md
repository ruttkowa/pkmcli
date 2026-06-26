# pkm

A local-first Personal Knowledge Management TUI — a navigation, organization, and knowledge management layer built around Markdown files. Not an editor; editing is delegated to your `$EDITOR`.

Inspired by Neovim, Zellij, LazyGit, and Obsidian.

---

## Build

Requires Go 1.26+ (install via `mise install go`).

```sh
go build -o pkm ./cmd/pkm
```

## Run

```sh
./pkm [vault-path]
```

`vault-path` defaults to `.`. If no argument is given, pkm prompts for a path before launching. On first run, the directory is initialized with the required structure:

```
vault/
├── notes/        ← all Markdown notes live here
├── templates/    ← optional note templates
└── .pkm/
    ├── config.toml
    ├── index.db  ← SQLite full-text + tag + link index
    └── session.yaml
```

---

## Keybindings

Active hotkeys are always shown in the **tooltip bar** at the bottom of the screen (Zellij-style). The bar is context-sensitive — it updates based on which pane is active and which view is open.

| Key | Action |
|-----|--------|
| `Tab` | Switch focus between Sidebar and Main area |
| `j` / `↓` | Move cursor down / scroll |
| `k` / `↑` | Move cursor up / scroll |
| `←` | Collapse section (sidebar) · Move cursor to section header (on a note row) |
| `→` | Expand section (sidebar) |
| `Enter` | Expand/collapse section (sidebar) · Open note (list/sidebar note title) |
| `e` | Open current note in `$EDITOR` |
| `Esc` / `Backspace` | Go back in history (or back to note list) |
| `[` / `Alt+Left` | Navigate back in pane history |
| `]` / `Alt+Right` | Navigate forward in pane history |
| `Ctrl+W` | Cycle focus to next split pane |
| `Ctrl+P` | Open pane picker — then `←`/`→` to select, `Enter` to confirm, `Esc` to cancel |
| `:` | Open command input (bottom bar) |
| `q` / `Ctrl+C` | Quit (saves session) |

**Mouse:** Left-click to focus a sidebar section, a split pane, or a note in the list.

---

## Command Input

Press `:` to open the command input at the **bottom** of the screen. A dropdown of matching commands appears above it as you type.

- `↑` / `↓` — cycle through suggestions
- `Tab` or `→` — complete the highlighted command and add a space for the next argument
- `Enter` — run the command
- `Esc` — cancel

| Command | Description |
|---------|-------------|
| `:new "Title"` | Create a new note in Inbox |
| `:open <query>` | Open a note by title (partial match) |
| `:search <query>` | Full-text search (title + body) |
| `:search #tag` | Search by tag |
| `:move <note> → <state>` | Move a note to a different state |
| `:archive <note>` | Archive a note |
| `:split [note]` | Open a new split pane (optionally showing a note) |
| `:close` | Close the focused split pane |
| `:theme dark\|light` | Switch color theme (persisted across sessions) |

Valid states for `:move`: `inbox`, `projects`, `areas`, `research`, `archive`.

---

## Knowledge Model

Follows a PARA-inspired workflow. All notes start in **Inbox** and must be explicitly promoted.

```
Inbox
├──► Projects  (max 4 active)
│       └──► Archive
├──► Areas
│       └──► Archive
└──► Research
        └──► Archive
```

---

## Note Format

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

Note body goes here. [[Wikilinks]] are supported.
```

---

## Templates

Place any `.md` file in `vault/templates/`. The first template found is applied when creating a new note. Supported variables:

| Variable | Value |
|----------|-------|
| `{{id}}` | Generated timestamp ID |
| `{{title}}` | Note title |
| `{{created}}` | Creation timestamp |
| `{{updated}}` | Update timestamp |

Example template (`templates/default.md`):
```markdown
---
id: {{id}}
title: {{title}}
created: {{created}}
updated: {{updated}}
state: inbox
tags: []
---

## {{title}}

```

---

## Links and Backlinks

Wikilink syntax is supported in note bodies:

```
[[Docker]]
[[Docker|Container Runtime]]
```

- Links to non-existing notes are valid — the note is created when opened.
- In read mode, `[[Docker|Container Runtime]]` renders as `Container Runtime`.
- The viewer shows backlinks (other notes that link to the current note) at the bottom.

---

## Search

- `:search docker` — full-text search across titles and note bodies (SQLite FTS5)
- `:search #linux` — tag search

---

## Session Restore

On quit, the current note and active sidebar section are saved to `.pkm/session.yaml`. The next launch restores them automatically.

---

## File Watcher

Changes to files in `vault/notes/` are detected automatically via fsnotify. The index and viewer update without requiring a restart — edits made in an external editor are reflected live.

---

## Pane Splits

Open multiple notes side by side with `:split [note]`. Each pane has its own independent navigation history (`[` / `]` to navigate). `Ctrl+W` cycles focus between panes. `:close` removes the focused pane.

---

## Running Tests

```sh
go test ./...
```

---

## Not Yet Implemented

- Mouse click-to-open for wikilinks in the viewer
- Git status overlay
- Backlinks panel in the viewer (index is built; UI display pending)
- Query language / saved searches
