# outcrawl Spec

## Goals

- Build a local-first Outline crawler.
- Mirror collections, document metadata, and document Markdown.
- Store normalized records in SQLite.
- Preserve raw API records for future re-rendering.
- Render normalized Markdown into an organized file tree.
- Support fast local text search.
- Avoid deleting local knowledge automatically when remote visibility changes.

## Product summary

`outcrawl` is a Go CLI that turns Outline workspace memory into a local SQLite archive plus normalized Markdown files.

V1 scope:

- official Outline API sync
- collections and documents
- incremental document body fetch based on `updatedAt`
- SHA-256 content hashes
- SQLite FTS5 search over document title/body
- archive status and doctor commands
- Markdown export by default after sync
- optional reuse of existing `ol` CLI credentials

Out of scope for V1:

- write-back actions
- attachment blob mirroring
- comment/history mirroring
- permission modeling
- automatic deletion/pruning
- desktop cache ingestion

## Data source

API sync uses `OUTLINE_API_TOKEN` and `OUTLINE_BASE_URL` by default. If no token is set, it may reuse the selected `ol` account.

Sync must:

1. list collections
2. list documents with pagination
3. compare remote `updatedAt` with local state
4. fetch full document Markdown only for new or changed documents
5. hash fetched Markdown
6. upsert metadata and raw API payloads
7. mark documents not seen during a full scan as `missing`
8. update `last_sync_at` only after the scan completes
9. export Markdown unless `--no-export` is set

## SQLite archive

SQLite is canonical. Markdown is generated output.

Core tables:

- `meta`
- `collections`
- `documents`
- `document_fts`

Document rows store:

- stable Outline id
- title, URL, URL id
- collection id and parent document id
- raw Markdown text
- SHA-256 content hash
- created/updated/published/archive/delete timestamps
- raw JSON
- sync timestamps
- missing flag
- exported relative path

## Markdown export

Markdown export writes active, non-missing documents under the configured archive directory.

Path shape:

```text
<collection>/<ancestor-title--id>/<title--id>.md
```

Each file includes frontmatter with stable Outline metadata followed by the raw Markdown body returned by Outline.

## Change detection

`updatedAt` is the primary remote change signal. The content hash is the local verification signal and avoids rewriting unchanged Markdown exports.

A full metadata scan is the default because it catches new docs, moves, renamed docs, and missing docs. Future versions may add an `updatedAfter` fast path if Outline exposes reliable filtering for the target instances.
