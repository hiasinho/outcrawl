# outcrawl

`outcrawl` mirrors Outline wiki documents into local SQLite and normalized
Markdown so you can search, query, diff, and read your Outline memory without
calling the Outline API for every question.

It has one ingestion path:

- `api`: official Outline API sync for collections and documents

SQLite is the canonical archive. Markdown is the durable human/agent surface.
Documents that disappear from a later API listing are marked missing instead of
being deleted automatically.

## Current Scope

- local SQLite storage with FTS5
- official Outline collection and document ingestion
- incremental sync based on document `updatedAt`
- SHA-256 content hashes for fetched Markdown
- normalized Markdown export organized by collection and parent document paths
- local full-text search over document title and body
- archive status and authentication diagnostics
- reuse of credentials already stored by the `ol` CLI

## Install

Build the CLI from a checkout:

```bash
go build -o ./outcrawl ./cmd/outcrawl
```

Or install it into any directory on your `PATH`:

```bash
go build -o ~/.local/bin/outcrawl ./cmd/outcrawl
```

## Quick Start

Use the selected `ol` account:

```bash
outcrawl init
outcrawl doctor
outcrawl status
outcrawl sync
outcrawl search "launch plan"
```

Or use explicit Outline API credentials:

```bash
export OUTLINE_BASE_URL="https://outline.example.com"
export OUTLINE_API_TOKEN="ol_api_token"
outcrawl sync
```

Default paths:

- config: `~/.outcrawl/config.toml`
- database: `~/.outcrawl/outcrawl.db`
- cache: `~/.outcrawl/cache`
- Markdown archive: `~/.outcrawl/pages`

## Commands

- `init` writes a starter config
- `doctor` checks config, auth, and Outline API access
- `status` prints archive counts and last sync time
- `sync` ingests Outline collections/documents and exports Markdown
- `sync --no-export` updates SQLite without rendering Markdown
- `export-md` renders normalized Markdown files from SQLite
- `search` searches document title and body text through FTS5

`doctor --json`, `status --json`, `sync --json`, `export-md --json`, and
`search --json` expose machine-readable payloads for agents and automation.

## Safety Model

`outcrawl` is read-only against Outline. It stores raw API payloads alongside
normalized rows so renderers can improve without recrawling.

Documents missing from a later API listing are preserved and marked `missing`
because permission changes, archival state, and API visibility gaps are not
reliable deletion signals. Local Markdown files are not pruned automatically.

Attachment blob mirroring is intentionally out of scope. Markdown keeps the
links returned by Outline, but `outcrawl` does not download image or file bytes
into the archive.

Secrets are never exported into Markdown.
