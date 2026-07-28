# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **PKM TUI** (Personal Knowledge Management Terminal UI) application — a navigation, organization and knowledge management layer built around Markdown files. It is **not a text editor**; editing is delegated to an external editor (nvim, vim, helix, emacs). Inspired by Neovim, Zellij, LazyGit, and Obsidian.

Full specification: `spec.html`

## Core Principles

- **Local-first, Markdown-only, Git-first**
- **No AI integration** — this is a non-goal, do not add it
- **Keyboard-first, Mouse-capable**
- No plugin system, no graph view, no query language, no embedded editor in v1

**Approved departure (2026-07-11):** read-only GitLab Issues may be shown
virtually in Tasks and cached at `.pkm/issues.json`. Authentication comes
only from `PKM_GITLAB_TOKEN`; issues never become notes or enter the note
index. Everything else remains local-first.

## Knowledge Management Model (PARA-inspired)

States: **Inbox → Projects / Areas / Research → Archive**

Rules:
- All new notes land in Inbox
- Notes must be explicitly promoted to another state
- Maximum 4 active projects
- Archive is the final inactive state

## Storage Format

**Filename:** `<ID> <Title>.md` — e.g., `202606241530 Docker.md`

**Frontmatter:**
```yaml
---
id: 202606241530
title: Docker
created: 2026-06-24T15:30:00
updated: 2026-06-24T15:45:00
state: research
project:
tags:
  - linux
  - containers
---
```

**Vault structure:**
```
vault/
├── notes/
├── templates/
├── .pkm/
│   ├── config.toml
│   ├── projects.toml
│   └── index.db       ← SQLite for indexes
└── .git/
```

No content-based folder hierarchy — organization is through metadata, tags, and states.

## UI Layout

```
┌──────────────────────────────────────────────┐
│ Breadcrumb / Context Bar                     │
├──────────────┬───────────────────────────────┤
│ Sidebar 25%  │ Main Pane 75%                 │
│ (Inbox,      │ (Markdown viewer, Backlinks,  │
│  Projects,   │  Tags, Tasks)                 │
│  Areas, ...)  │                              │
└──────────────┴───────────────────────────────┘
```

Pane system supports horizontal splits, resizable panes (mouse + keyboard), per-pane navigation history (back/forward like a browser).

## Command System

Activated via `:` — command palette with fuzzy autocomplete.

**DSL general form:** `:<command> [target] [arguments] [→ destination]`

Examples: `:new "Docker"`, `:open docker`, `:move docker → project/homelab`, `:archive docker`, `:split docker`

## Architecture: State-Driven Pipeline

All commands flow through this pipeline:

```
Input → Parser → Resolver → Command → State Update → Event → UI Refresh
```

Commands mutate state. State emits events. UI reacts to state changes.

## Index Architecture

Hybrid model using SQLite (`.pkm/index.db`):
- **Startup:** load persisted index, then run validation scan
- **Runtime:** file watcher → incremental updates → persistent cache

Indexes: full-text, tag, link, backlink, metadata

## Links

Supported syntax: `[[Docker]]`, `[[Docker|Container Runtime]]`
- Links to non-existing notes are valid; missing notes are created when opened
- Read mode renders alias only: `Container Runtime`

## Search (v1)

Fuzzy search, title search, content search, tag search. No query language in v1.

## Tasks

Markdown checkboxes (`- [ ]` / `- [x]`). Plain checkboxes with no metadata are always valid — nothing is rewritten on load, only on an actual toggle. Tasks may be indexed by the system.

**Approved departure (2026-07-10):** a checkbox line may optionally carry a completion date and a result, canonical order `text ✅ YYYY-MM-DD --> result`. The date is stamped on toggle-to-done and stripped on toggle-to-undone (not accumulated); a result is free text after `-->`, rendered/opened as a link if it's a `[[wikilink]]`. This is a narrow, explicitly user-approved extension — it is not a general "custom task language" or query language (still a non-goal below). See `spec.html` Tasks section and `internal/tui/viewer.go` (`parseTaskLine`/`formatTaskLine`/`toggleCheckboxLine`).

## Templates

Stored as Markdown files in `vault/templates/`. Supported variables: `{{id}}`, `{{title}}`, `{{created}}`, `{{updated}}`

## Application Startup

On launch: restore previous session, restore previous pane layout, restore last opened note, open in read mode.

## Explicit Non-Goals (v1)

AI integration, attachment management, graph visualization, plugin system, embedded editor, canvas, PDF/image management, custom task language, query language.
