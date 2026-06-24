package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiasinho/outcrawl/internal/outlineapi"
	"github.com/hiasinho/outcrawl/internal/store"
)

func TestExportRemovesOldFileWhenDocumentPathChanges(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	doc := outlineapi.Document{
		ID:           "doc-1",
		Title:        "Original Title",
		Text:         "Current content",
		CollectionID: "col-1",
		UpdatedAt:    "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentFull(ctx, doc, store.HashText(doc.Text), 100); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	oldPath := exportedPath(t, ctx, st, dir, doc.ID)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected initial export at %s: %v", oldPath, err)
	}

	doc.Title = "Renamed Title"
	doc.UpdatedAt = "2026-06-24T14:00:00Z"
	if err := st.UpsertDocumentFull(ctx, doc, store.HashText(doc.Text), 200); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}

	newPath := exportedPath(t, ctx, st, dir, doc.ID)
	if newPath == oldPath {
		t.Fatalf("path did not change after rename: %s", newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected renamed export at %s: %v", newPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old export still exists, stat error = %v", err)
	}
}

func TestExportRemovesDuplicateFileWithSameDocumentID(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	doc := outlineapi.Document{
		ID:           "doc-1",
		Title:        "Canonical Title",
		Text:         "Current content",
		CollectionID: "col-1",
		UpdatedAt:    "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentFull(ctx, doc, store.HashText(doc.Text), 100); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	canonicalPath := exportedPath(t, ctx, st, dir, doc.ID)
	stalePath := filepath.Join(dir, "Stale-Title--doc-1.md")
	if stalePath == canonicalPath {
		t.Fatalf("test stale path unexpectedly matched canonical path: %s", stalePath)
	}
	if err := os.WriteFile(stalePath, []byte("---\nid: \"doc-1\"\ntitle: \"Stale Title\"\n---\n\n# stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("expected canonical export at %s: %v", canonicalPath, err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("duplicate stale export still exists, stat error = %v", err)
	}
}

func TestExportRemovesNormalTreeFileForMissingDocument(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "outcrawl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	doc := outlineapi.Document{
		ID:           "doc-1",
		Title:        "Missing Later",
		Text:         "Current content",
		CollectionID: "col-1",
		UpdatedAt:    "2026-06-24T13:00:00Z",
	}
	if err := st.UpsertDocumentFull(ctx, doc, store.HashText(doc.Text), 100); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}

	docs, err := st.Documents(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("exported docs = %d, want 1", len(docs))
	}
	exportedPath := filepath.Join(dir, docs[0].Path)
	if _, err := os.Stat(exportedPath); err != nil {
		t.Fatalf("expected initial export at %s: %v", exportedPath, err)
	}

	if err := st.MarkMissingNotSeen(ctx, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(ctx, st, dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(exportedPath); !os.IsNotExist(err) {
		t.Fatalf("missing document export still exists, stat error = %v", err)
	}
}

func exportedPath(t *testing.T, ctx context.Context, st *store.Store, dir, id string) string {
	t.Helper()
	docs, err := st.Documents(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		if doc.ID == id {
			return filepath.Join(dir, doc.Path)
		}
	}
	t.Fatalf("document %q not found", id)
	return ""
}
