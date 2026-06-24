package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"

	"github.com/hiasinho/outcrawl/internal/outlineapi"
)

const schemaVersion = 1

type Store struct {
	db   *sql.DB
	path string
}

type DocumentMeta struct {
	ID        string
	UpdatedAt string
	Hash      string
}

type Document struct {
	ID               string
	Title            string
	Text             string
	URL              string
	URLID            string
	CollectionID     string
	ParentDocumentID string
	ContentHash      string
	Path             string
	CreatedAt        string
	UpdatedAt        string
	PublishedAt      string
	ArchivedAt       string
	DeletedAt        string
	Missing          bool
}

type Collection struct {
	ID   string
	Name string
}

type Status struct {
	DBPath      string `json:"db_path"`
	DBBytes     int64  `json:"db_bytes"`
	Collections int    `json:"collections"`
	Documents   int    `json:"documents"`
	Missing     int    `json:"missing"`
	LastSyncAt  int64  `json:"last_sync_at"`
}

type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
	Text  string `json:"text"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: path}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type txKey struct{}

func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx.ExecContext(ctx, q, args...)
	}
	return s.db.ExecContext(ctx, q, args...)
}

func (s *Store) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx.QueryContext(ctx, q, args...)
	}
	return s.db.QueryContext(ctx, q, args...)
}

func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx.QueryRowContext(ctx, q, args...)
	}
	return s.db.QueryRowContext(ctx, q, args...)
}

func (s *Store) init(ctx context.Context) error {
	stmts := []string{
		`pragma foreign_keys = on`,
		`pragma journal_mode = wal`,
		`pragma synchronous = normal`,
		`pragma temp_store = memory`,
		`pragma busy_timeout = 5000`,
		`create table if not exists meta (key text primary key, value text not null)`,
		`create table if not exists collections (
			id text primary key,
			name text not null,
			description text,
			color text,
			private integer not null default 0,
			created_at text,
			updated_at text,
			raw_json text,
			synced_at integer not null,
			last_seen_at integer not null,
			missing integer not null default 0
		)`,
		`create table if not exists documents (
			id text primary key,
			title text not null,
			text text not null default '',
			url text,
			url_id text,
			collection_id text,
			parent_document_id text,
			content_hash text,
			path text,
			created_at text,
			updated_at text,
			published_at text,
			archived_at text,
			deleted_at text,
			raw_json text,
			synced_at integer not null,
			last_seen_at integer not null,
			missing integer not null default 0
		)`,
		`create index if not exists documents_collection_id on documents(collection_id)`,
		`create index if not exists documents_parent_document_id on documents(parent_document_id)`,
		`create index if not exists documents_updated_at on documents(updated_at desc)`,
		`create index if not exists documents_last_seen_at on documents(last_seen_at desc)`,
		`create virtual table if not exists document_fts using fts5(id unindexed, title, text, path unindexed)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `insert into meta(key, value) values('schema_version', ?) on conflict(key) do update set value=excluded.value`, fmt.Sprint(schemaVersion)); err != nil {
		return err
	}
	return nil
}

func (s *Store) UpsertCollection(ctx context.Context, c outlineapi.Collection, syncAt int64) error {
	raw := string(c.Raw)
	_, err := s.exec(ctx, `insert into collections(id, name, description, color, private, created_at, updated_at, raw_json, synced_at, last_seen_at, missing)
		values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		on conflict(id) do update set
		name=excluded.name, description=excluded.description, color=excluded.color, private=excluded.private,
		created_at=excluded.created_at, updated_at=excluded.updated_at, raw_json=excluded.raw_json,
		synced_at=excluded.synced_at, last_seen_at=excluded.last_seen_at, missing=0`,
		c.ID, c.Name, c.Description, c.Color, boolInt(c.Private), c.CreatedAt, c.UpdatedAt, raw, syncAt, syncAt)
	return err
}

func (s *Store) UpsertDocumentSummary(ctx context.Context, d outlineapi.Document, syncAt int64) error {
	raw := string(d.Raw)
	_, err := s.exec(ctx, `insert into documents(id, title, url, url_id, collection_id, parent_document_id, created_at, updated_at, published_at, archived_at, deleted_at, raw_json, synced_at, last_seen_at, missing)
		values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		on conflict(id) do update set
		title=excluded.title, url=excluded.url, url_id=excluded.url_id, collection_id=excluded.collection_id,
		parent_document_id=excluded.parent_document_id, created_at=excluded.created_at, updated_at=excluded.updated_at,
		published_at=excluded.published_at, archived_at=excluded.archived_at, deleted_at=excluded.deleted_at,
		raw_json=excluded.raw_json, synced_at=excluded.synced_at, last_seen_at=excluded.last_seen_at, missing=0`,
		d.ID, fallbackTitle(d.Title), d.URL, d.URLID, d.CollectionID, d.ParentDocumentID, d.CreatedAt, d.UpdatedAt, d.PublishedAt, d.ArchivedAt, d.DeletedAt, raw, syncAt, syncAt)
	return err
}

func (s *Store) UpsertDocumentFull(ctx context.Context, d outlineapi.Document, hash string, syncAt int64) error {
	if err := s.UpsertDocumentSummary(ctx, d, syncAt); err != nil {
		return err
	}
	raw := string(d.Raw)
	_, err := s.exec(ctx, `update documents set text=?, content_hash=?, raw_json=?, synced_at=?, last_seen_at=?, missing=0 where id=?`, d.Text, hash, raw, syncAt, syncAt, d.ID)
	if err != nil {
		return err
	}
	return s.ReindexDocument(ctx, d.ID)
}

func (s *Store) ReindexDocument(ctx context.Context, id string) error {
	var title, text, path string
	if err := s.queryRow(ctx, `select title, text, coalesce(path, '') from documents where id=?`, id).Scan(&title, &text, &path); err != nil {
		return err
	}
	if _, err := s.exec(ctx, `delete from document_fts where id=?`, id); err != nil {
		return err
	}
	_, err := s.exec(ctx, `insert into document_fts(id, title, text, path) values(?, ?, ?, ?)`, id, title, text, path)
	return err
}

func (s *Store) ExistingDocument(ctx context.Context, id string) (DocumentMeta, bool, error) {
	var meta DocumentMeta
	err := s.queryRow(ctx, `select id, coalesce(updated_at, ''), coalesce(content_hash, '') from documents where id=?`, id).Scan(&meta.ID, &meta.UpdatedAt, &meta.Hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return DocumentMeta{}, false, nil
		}
		return DocumentMeta{}, false, err
	}
	return meta, true, nil
}

func (s *Store) MarkMissingNotSeen(ctx context.Context, syncAt int64) error {
	if _, err := s.exec(ctx, `update collections set missing=1, synced_at=? where last_seen_at < ?`, syncAt, syncAt); err != nil {
		return err
	}
	_, err := s.exec(ctx, `update documents set missing=1, synced_at=? where last_seen_at < ?`, syncAt, syncAt)
	return err
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.exec(ctx, `insert into meta(key, value) values(?, ?) on conflict(key) do update set value=excluded.value`, key, value)
	return err
}

func (s *Store) Status(ctx context.Context) (Status, error) {
	st := Status{DBPath: s.path}
	if info, err := os.Stat(s.path); err == nil {
		st.DBBytes = info.Size()
	}
	counts := []struct {
		name string
		dst  *int
	}{
		{"collections", &st.Collections},
		{"documents", &st.Documents},
	}
	for _, c := range counts {
		if err := s.queryRow(ctx, `select count(*) from `+c.name).Scan(c.dst); err != nil {
			return st, err
		}
	}
	if err := s.queryRow(ctx, `select count(*) from documents where missing=1`).Scan(&st.Missing); err != nil {
		return st, err
	}
	var last string
	if err := s.queryRow(ctx, `select value from meta where key='last_sync_at'`).Scan(&last); err == nil {
		_, _ = fmt.Sscan(last, &st.LastSyncAt)
	}
	return st, nil
}

func (s *Store) Collections(ctx context.Context) (map[string]Collection, error) {
	rows, err := s.query(ctx, `select id, name from collections where missing=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Collection{}
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		out[c.ID] = c
	}
	return out, rows.Err()
}

func (s *Store) Documents(ctx context.Context, includeMissing bool) ([]Document, error) {
	q := `select id, title, text, coalesce(url, ''), coalesce(url_id, ''), coalesce(collection_id, ''), coalesce(parent_document_id, ''), coalesce(content_hash, ''), coalesce(path, ''), coalesce(created_at, ''), coalesce(updated_at, ''), coalesce(published_at, ''), coalesce(archived_at, ''), coalesce(deleted_at, ''), missing from documents`
	if !includeMissing {
		q += ` where missing=0`
	}
	q += ` order by title collate nocase`
	rows, err := s.query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []Document
	for rows.Next() {
		var d Document
		var missing int
		if err := rows.Scan(&d.ID, &d.Title, &d.Text, &d.URL, &d.URLID, &d.CollectionID, &d.ParentDocumentID, &d.ContentHash, &d.Path, &d.CreatedAt, &d.UpdatedAt, &d.PublishedAt, &d.ArchivedAt, &d.DeletedAt, &missing); err != nil {
			return nil, err
		}
		d.Missing = missing != 0
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func (s *Store) SetDocumentPath(ctx context.Context, id, path string) error {
	_, err := s.exec(ctx, `update documents set path=? where id=?`, path, id)
	if err != nil {
		return err
	}
	return s.ReindexDocument(ctx, id)
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	ftsQuery := plainTextFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.query(ctx, `select documents.id, documents.title, coalesce(documents.path, ''), snippet(document_fts, 2, '[', ']', ' ... ', 18)
		from document_fts
		join documents on documents.id = document_fts.id
		where document_fts match ? and documents.missing=0
		limit ?`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.Title, &r.Path, &r.Text); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func plainTextFTSQuery(query string) string {
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, `"`+b.String()+`"`)
		b.Reset()
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return strings.Join(tokens, " AND ")
}

func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func fallbackTitle(title string) string {
	if title == "" {
		return "Untitled"
	}
	return title
}

func RawJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func NowUnix() int64 { return time.Now().Unix() }
