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

Standard Markdown checkboxes only (`- [ ]` / `- [x]`). No custom task syntax. Tasks may be indexed by the system.

## Templates

Stored as Markdown files in `vault/templates/`. Supported variables: `{{id}}`, `{{title}}`, `{{created}}`, `{{updated}}`

## Application Startup

On launch: restore previous session, restore previous pane layout, restore last opened note, open in read mode.

## Explicit Non-Goals (v1)

AI integration, attachment management, graph visualization, plugin system, embedded editor, canvas, PDF/image management, custom task language, query language.
