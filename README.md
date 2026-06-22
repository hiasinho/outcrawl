# outcrawl

`outcrawl` mirrors Outline wiki documents into a local SQLite archive and normalized Markdown tree so coding agents can search and read local files instead of calling the Outline API for every question.

The tool is read-only against Outline. SQLite is the canonical local archive; Markdown is the durable human/agent surface.

## Current scope

- Outline API ingestion for collections and documents
- local SQLite storage with FTS5
- incremental sync based on document `updatedAt`
- SHA-256 content hashes for fetched Markdown
- safe missing-document marking instead of automatic deletion
- Markdown export organized by collection and parent document path
- local full-text search
- fallback to credentials already stored by the `ol` CLI

## Quick start

```bash
outcrawl init
outcrawl doctor
outcrawl sync
outcrawl search "launch plan"
```

Default paths:

- config: `~/.outcrawl/config.toml`
- database: `~/.outcrawl/outcrawl.db`
- cache: `~/.outcrawl/cache`
- Markdown archive: `~/.outcrawl/pages`

## Authentication

`outcrawl` first checks environment variables:

```bash
export OUTLINE_BASE_URL="https://outline.example.com"
export OUTLINE_API_TOKEN="ol_api_token"
```

If no token is present in the environment, it tries to reuse the selected `ol` account via:

```bash
ol auth token view
ol account current --json
```

## Commands

```bash
outcrawl init                 # write starter config
outcrawl doctor               # check paths, auth, and API access
outcrawl status               # show local archive counts
outcrawl sync                 # sync Outline into SQLite, then export Markdown
outcrawl sync --no-export     # sync SQLite only
outcrawl export-md            # render Markdown from SQLite
outcrawl search "query"       # search local FTS index
```

Most machine-facing commands support `--json`.

## Safety model

`outcrawl` never writes to Outline. Documents that disappear from a later sync are marked `missing` in SQLite rather than removed from the archive or filesystem. A future explicit `prune` command can handle deletion once the semantics are clear for permission changes, archived docs, and API visibility gaps.
