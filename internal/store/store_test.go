package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hiasinho/outcrawl/internal/outlineapi"
)

func TestSearchTreatsPunctuationAsPlainText(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	doc := outlineapi.Document{
		ID:        "doc-1",
		Title:     "Profiles, Prompts & Subagents",
		Text:      "Notes about CanvasReviewerWorker (alpha-beta): prompt safety.",
		UpdatedAt: "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentFull(ctx, doc, HashText(doc.Text), 100); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDocumentPath(ctx, doc.ID, "Profiles-Prompts-Subagents.md"); err != nil {
		t.Fatal(err)
	}

	queries := []string{
		`Profiles, Prompts & Subagents`,
		`CanvasReviewerWorker (alpha-beta): prompt "safety"`,
	}
	for _, query := range queries {
		results, err := st.Search(ctx, query, 20)
		if err != nil {
			t.Fatalf("Search(%q) returned error: %v", query, err)
		}
		if len(results) != 1 || results[0].ID != doc.ID {
			t.Fatalf("Search(%q) returned %#v, want doc %q", query, results, doc.ID)
		}
	}
}

func TestSearchExcludesMissingDocuments(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	current := outlineapi.Document{
		ID:        "current-doc",
		Title:     "Current CanvasReviewerWorker",
		Text:      "CanvasReviewerWorker is still exported.",
		UpdatedAt: "2026-06-24T13:00:00Z",
	}
	missing := outlineapi.Document{
		ID:        "missing-doc",
		Title:     "Missing CanvasReviewerWorker",
		Text:      "CanvasReviewerWorker should not appear by default.",
		UpdatedAt: "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentFull(ctx, current, HashText(current.Text), 100); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDocumentPath(ctx, current.ID, "current.md"); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDocumentFull(ctx, missing, HashText(missing.Text), 50); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDocumentPath(ctx, missing.ID, "missing.md"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkMissingNotSeen(ctx, 100); err != nil {
		t.Fatal(err)
	}

	results, err := st.Search(ctx, "CanvasReviewerWorker", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != current.ID {
		t.Fatalf("Search returned %#v, want only current doc %q", results, current.ID)
	}
}

func TestMarkMissingNotSeenUpdatesSyncedAtWithoutChangingLastSeenAt(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const firstSync int64 = 100
	const secondSync int64 = 200
	doc := outlineapi.Document{
		ID:        "doc-1",
		Title:     "Current in first sync",
		UpdatedAt: "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentSummary(ctx, doc, firstSync); err != nil {
		t.Fatal(err)
	}

	if err := st.MarkMissingNotSeen(ctx, secondSync); err != nil {
		t.Fatal(err)
	}

	var syncedAt, lastSeenAt int64
	var missing int
	if err := st.DB().QueryRowContext(ctx, `select synced_at, last_seen_at, missing from documents where id=?`, doc.ID).Scan(&syncedAt, &lastSeenAt, &missing); err != nil {
		t.Fatal(err)
	}

	if syncedAt != secondSync {
		t.Fatalf("synced_at = %d, want %d", syncedAt, secondSync)
	}
	if lastSeenAt != firstSync {
		t.Fatalf("last_seen_at = %d, want %d", lastSeenAt, firstSync)
	}
	if missing != 1 {
		t.Fatalf("missing = %d, want 1", missing)
	}
}
