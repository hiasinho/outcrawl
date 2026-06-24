package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/hiasinho/outcrawl/internal/store"
)

type ExportSummary struct {
	Documents int `json:"documents"`
	Written   int `json:"written"`
	Skipped   int `json:"skipped"`
}

func Export(ctx context.Context, st *store.Store, dir string) (ExportSummary, error) {
	collections, err := st.Collections(ctx)
	if err != nil {
		return ExportSummary{}, err
	}
	allDocs, err := st.Documents(ctx, true)
	if err != nil {
		return ExportSummary{}, err
	}
	if err := removeMissingExports(allDocs, dir); err != nil {
		return ExportSummary{}, err
	}
	docs := make([]store.Document, 0, len(allDocs))
	for _, d := range allDocs {
		if !d.Missing {
			docs = append(docs, d)
		}
	}
	byID := make(map[string]store.Document, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	summary := ExportSummary{Documents: len(docs)}
	sort.Slice(docs, func(i, j int) bool { return strings.ToLower(docs[i].Title) < strings.ToLower(docs[j].Title) })
	for _, doc := range docs {
		rel := documentPath(doc, byID, collections)
		abs := filepath.Join(dir, rel)
		body := render(doc)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return summary, err
		}
		if existing, err := os.ReadFile(abs); err == nil && string(existing) == body {
			summary.Skipped++
		} else {
			if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
				return summary, err
			}
			summary.Written++
		}
		if err := st.SetDocumentPath(ctx, doc.ID, rel); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func removeMissingExports(docs []store.Document, dir string) error {
	for _, doc := range docs {
		if !doc.Missing || strings.TrimSpace(doc.Path) == "" {
			continue
		}
		abs, ok := exportPath(dir, doc.Path)
		if !ok {
			continue
		}
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func exportPath(dir, rel string) (string, bool) {
	if filepath.IsAbs(rel) {
		return "", false
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(dir, clean), true
}

func documentPath(doc store.Document, docs map[string]store.Document, collections map[string]store.Collection) string {
	collectionName := "Unfiled"
	if col, ok := collections[doc.CollectionID]; ok && strings.TrimSpace(col.Name) != "" {
		collectionName = col.Name
	}
	parts := []string{safeSegment(collectionName, doc.CollectionID)}
	ancestors := ancestorSegments(doc, docs)
	parts = append(parts, ancestors...)
	parts = append(parts, safeSegment(doc.Title, doc.ID)+".md")
	return filepath.Join(parts...)
}

func ancestorSegments(doc store.Document, docs map[string]store.Document) []string {
	var rev []string
	seen := map[string]bool{doc.ID: true}
	parentID := doc.ParentDocumentID
	for parentID != "" {
		if seen[parentID] {
			break
		}
		seen[parentID] = true
		parent, ok := docs[parentID]
		if !ok {
			break
		}
		rev = append(rev, safeSegment(parent.Title, parent.ID))
		parentID = parent.ParentDocumentID
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func render(doc store.Document) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %q\n", doc.ID)
	fmt.Fprintf(&b, "title: %q\n", doc.Title)
	fmt.Fprintf(&b, "url: %q\n", doc.URL)
	fmt.Fprintf(&b, "url_id: %q\n", doc.URLID)
	fmt.Fprintf(&b, "collection_id: %q\n", doc.CollectionID)
	fmt.Fprintf(&b, "parent_document_id: %q\n", doc.ParentDocumentID)
	fmt.Fprintf(&b, "created_at: %q\n", doc.CreatedAt)
	fmt.Fprintf(&b, "updated_at: %q\n", doc.UpdatedAt)
	fmt.Fprintf(&b, "content_hash: %q\n", doc.ContentHash)
	b.WriteString("---\n\n")
	if strings.TrimSpace(doc.Text) == "" {
		fmt.Fprintf(&b, "# %s\n", doc.Title)
		return b.String()
	}
	b.WriteString(strings.TrimRight(doc.Text, "\n"))
	b.WriteString("\n")
	return b.String()
}

var dashRuns = regexp.MustCompile(`-+`)

func safeSegment(title, id string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == ':' || r == '.':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(dashRuns.ReplaceAllString(b.String(), "-"), "-")
	if slug == "" {
		slug = "Untitled"
	}
	if len([]rune(slug)) > 80 {
		runes := []rune(slug)
		slug = string(runes[:80])
		slug = strings.Trim(slug, "-")
	}
	return slug + "--" + shortID(id)
}

func shortID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "unknown"
	}
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
