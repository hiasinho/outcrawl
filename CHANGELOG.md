# Changelog

All notable changes to outcrawl are documented in this file.

## [0.0.3] - 2026-06-24

### Maintenance

- Added tracked changelog release notes for the accumulated fixes since `v0.0.1`.
- Verified there are no open GitHub issues requiring code changes for this release.

### Tests

- Ran the Go test suite before publishing the release.

## [0.0.2] - 2026-06-24

### Fixed

- Keep `last_seen_at` distinct from sync processing time when rows are marked missing. Missing documents and collections now advance `synced_at` without changing `last_seen_at`.
- Remove existing normal-tree Markdown exports for documents marked missing so missing documents are no longer presented as current files.
- Clean up stale Markdown exports when a document's canonical export path changes, including duplicate files with the same frontmatter `id`.

### Tests

- Added regression coverage for missing document timestamp semantics.
- Added Markdown export cleanup coverage for missing documents, renamed document paths, and duplicate stale exports.

## [0.0.1] - 2026-06-22

Initial outcrawl release.

- Sync Outline collections and documents into local SQLite.
- Export normalized Markdown by default.
- Search local document content with SQLite FTS5.
- Reuse credentials from environment variables or the selected `ol` account.
- Preserve missing documents instead of deleting local archive data.

[0.0.3]: https://github.com/hiasinho/outcrawl/compare/v0.0.1...v0.0.3
[0.0.2]: https://github.com/hiasinho/outcrawl/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/hiasinho/outcrawl/releases/tag/v0.0.1
