package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hiasinho/outcrawl/internal/outlineapi"
)

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
