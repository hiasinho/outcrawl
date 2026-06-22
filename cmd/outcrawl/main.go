package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hiasinho/outcrawl/internal/config"
	"github.com/hiasinho/outcrawl/internal/markdown"
	"github.com/hiasinho/outcrawl/internal/outlineapi"
	"github.com/hiasinho/outcrawl/internal/store"
)

var version = "dev"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "outcrawl:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	global, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if global.version {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		printHelp(stdout)
		return nil
	}
	cmd, cmdArgs := rest[0], rest[1:]
	if cmd == "init" {
		return runInit(stdout, global.config)
	}
	cfg, err := config.Load(global.config)
	if err != nil {
		return err
	}
	if global.db != "" {
		cfg.DBPath = config.ExpandPath(global.db)
	}
	switch cmd {
	case "doctor":
		return runDoctor(ctx, stdout, cfg, cmdArgs)
	case "status":
		return runStatus(ctx, stdout, cfg, cmdArgs)
	case "sync":
		return runSync(ctx, stdout, stderr, cfg, cmdArgs)
	case "export-md":
		return runExportMarkdown(ctx, stdout, cfg, cmdArgs)
	case "search":
		return runSearch(ctx, stdout, cfg, cmdArgs)
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

type globalArgs struct {
	config  string
	db      string
	version bool
}

func parseGlobal(args []string) (globalArgs, []string, error) {
	var g globalArgs
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--version" || arg == "-V":
			g.version = true
			args = args[1:]
		case arg == "--config":
			if len(args) < 2 {
				return g, nil, errors.New("--config requires a value")
			}
			g.config = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--config="):
			g.config = strings.TrimPrefix(arg, "--config=")
			args = args[1:]
		case arg == "--db":
			if len(args) < 2 {
				return g, nil, errors.New("--db requires a value")
			}
			g.db = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--db="):
			g.db = strings.TrimPrefix(arg, "--db=")
			args = args[1:]
		case strings.HasPrefix(arg, "-"):
			return g, nil, fmt.Errorf("unknown global flag %s", arg)
		default:
			return g, args, nil
		}
	}
	return g, args, nil
}

func runInit(stdout io.Writer, configPath string) error {
	path, err := config.WriteStarter(configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	return nil
}

func runDoctor(ctx context.Context, stdout io.Writer, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report := map[string]any{
		"db_path":       cfg.DBPath,
		"cache_dir":     cfg.CacheDir,
		"markdown_dir":  cfg.MarkdownDir,
		"config_path":   config.DefaultPath(),
		"token_env":     cfg.Outline.TokenEnv,
		"token_present": cfg.TokenFromEnv() != "",
		"base_url":      firstNonEmpty(cfg.BaseURLFromEnv(), cfg.Outline.BaseURL),
		"ol_available":  commandAvailable("ol"),
	}
	auth, err := outlineapi.ResolveAuth(ctx, cfg)
	if err != nil {
		report["auth_ok"] = false
		report["auth_error"] = err.Error()
	} else {
		report["auth_ok"] = true
		report["auth_source"] = auth.Source
		report["base_url"] = auth.BaseURL
		user, err := outlineapi.New(auth).AuthInfo(ctx)
		if err != nil {
			report["api_ok"] = false
			report["api_error"] = err.Error()
		} else {
			report["api_ok"] = true
			report["user"] = user
		}
	}
	if *jsonOut {
		return writeJSON(stdout, report)
	}
	fmt.Fprintf(stdout, "db: %s\n", report["db_path"])
	fmt.Fprintf(stdout, "markdown: %s\n", report["markdown_dir"])
	fmt.Fprintf(stdout, "ol available: %v\n", report["ol_available"])
	fmt.Fprintf(stdout, "auth ok: %v\n", report["auth_ok"])
	if report["auth_error"] != nil {
		fmt.Fprintf(stdout, "auth error: %s\n", report["auth_error"])
	}
	if report["api_ok"] != nil {
		fmt.Fprintf(stdout, "api ok: %v\n", report["api_ok"])
	}
	return nil
}

func runStatus(ctx context.Context, stdout io.Writer, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	status, err := st.Status(ctx)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "db: %s (%d bytes)\n", status.DBPath, status.DBBytes)
	fmt.Fprintf(stdout, "collections: %d\n", status.Collections)
	fmt.Fprintf(stdout, "documents: %d\n", status.Documents)
	fmt.Fprintf(stdout, "missing: %d\n", status.Missing)
	if status.LastSyncAt > 0 {
		fmt.Fprintf(stdout, "last sync: %s\n", time.Unix(status.LastSyncAt, 0).Format(time.RFC3339))
	} else {
		fmt.Fprintln(stdout, "last sync: never")
	}
	return nil
}

type syncSummary struct {
	Collections int                     `json:"collections"`
	Documents   int                     `json:"documents"`
	Fetched     int                     `json:"fetched"`
	Unchanged   int                     `json:"unchanged"`
	Missing     int                     `json:"missing"`
	Export      *markdown.ExportSummary `json:"export,omitempty"`
}

func runSync(ctx context.Context, stdout, stderr io.Writer, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Bool("export", true, "export Markdown after syncing (default)")
	noExport := fs.Bool("no-export", false, "skip Markdown export")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := config.EnsureDirs(cfg); err != nil {
		return err
	}
	progressf(stderr, "authenticating")
	auth, err := outlineapi.ResolveAuth(ctx, cfg)
	if err != nil {
		return err
	}
	progressf(stderr, "using auth source: %s", auth.Source)
	client := outlineapi.New(auth)
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	progressf(stderr, "listing collections")
	collections, err := client.ListCollections(ctx)
	if err != nil {
		return err
	}
	progressf(stderr, "listing documents")
	docs, err := client.ListDocuments(ctx, cfg.Outline.PageSize)
	if err != nil {
		return err
	}
	progressf(stderr, "found %d collections and %d documents", len(collections), len(docs))
	syncAt := time.Now().Unix()
	summary := syncSummary{Collections: len(collections), Documents: len(docs)}
	for _, col := range collections {
		if err := st.UpsertCollection(ctx, col, syncAt); err != nil {
			return err
		}
	}
	for i, listed := range docs {
		existing, ok, err := st.ExistingDocument(ctx, listed.ID)
		if err != nil {
			return err
		}
		if err := st.UpsertDocumentSummary(ctx, listed, syncAt); err != nil {
			return err
		}
		needsFetch := !ok || existing.UpdatedAt != listed.UpdatedAt || existing.Hash == ""
		if !needsFetch {
			summary.Unchanged++
			continue
		}
		if summary.Fetched == 0 || summary.Fetched%25 == 0 {
			progressf(stderr, "fetching changed documents: %d fetched, %d/%d scanned", summary.Fetched, i+1, len(docs))
		}
		full, err := client.GetDocument(ctx, listed.ID)
		if err != nil {
			return err
		}
		full = mergeDocument(listed, full)
		hash := store.HashText(full.Text)
		if err := st.UpsertDocumentFull(ctx, full, hash, syncAt); err != nil {
			return err
		}
		summary.Fetched++
	}
	if err := st.MarkMissingNotSeen(ctx, syncAt); err != nil {
		return err
	}
	if err := st.SetMeta(ctx, "last_sync_at", fmt.Sprint(syncAt)); err != nil {
		return err
	}
	status, err := st.Status(ctx)
	if err == nil {
		summary.Missing = status.Missing
	}
	if !*noExport {
		progressf(stderr, "exporting Markdown")
		exportSummary, err := markdown.Export(ctx, st, cfg.MarkdownDir)
		if err != nil {
			return err
		}
		summary.Export = &exportSummary
	}
	if *jsonOut {
		return writeJSON(stdout, summary)
	}
	fmt.Fprintf(stdout, "collections: %d\n", summary.Collections)
	fmt.Fprintf(stdout, "documents: %d (%d fetched, %d unchanged)\n", summary.Documents, summary.Fetched, summary.Unchanged)
	fmt.Fprintf(stdout, "missing: %d\n", summary.Missing)
	if summary.Export != nil {
		fmt.Fprintf(stdout, "exported: %d written, %d skipped\n", summary.Export.Written, summary.Export.Skipped)
	}
	_ = stderr
	return nil
}

func runExportMarkdown(ctx context.Context, stdout io.Writer, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("export-md", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.MarkdownDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	summary, err := markdown.Export(ctx, st, cfg.MarkdownDir)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(stdout, summary)
	}
	fmt.Fprintf(stdout, "documents: %d\n", summary.Documents)
	fmt.Fprintf(stdout, "written: %d\n", summary.Written)
	fmt.Fprintf(stdout, "skipped: %d\n", summary.Skipped)
	fmt.Fprintf(stdout, "dir: %s\n", cfg.MarkdownDir)
	return nil
}

func runSearch(ctx context.Context, stdout io.Writer, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	limit := fs.Int("limit", 20, "max results")
	if err := fs.Parse(args); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("search requires a query")
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	results, err := st.Search(ctx, query, *limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(stdout, results)
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "%s\n  %s\n  %s\n\n", r.Title, r.Path, r.Text)
	}
	return nil
}

func mergeDocument(listed, full outlineapi.Document) outlineapi.Document {
	if full.ID == "" {
		full.ID = listed.ID
	}
	if full.Title == "" {
		full.Title = listed.Title
	}
	if full.CollectionID == "" {
		full.CollectionID = listed.CollectionID
	}
	if full.ParentDocumentID == "" {
		full.ParentDocumentID = listed.ParentDocumentID
	}
	if full.URL == "" {
		full.URL = listed.URL
	}
	if full.URLID == "" {
		full.URLID = listed.URLID
	}
	if full.CreatedAt == "" {
		full.CreatedAt = listed.CreatedAt
	}
	if full.UpdatedAt == "" {
		full.UpdatedAt = listed.UpdatedAt
	}
	if full.PublishedAt == "" {
		full.PublishedAt = listed.PublishedAt
	}
	if full.ArchivedAt == "" {
		full.ArchivedAt = listed.ArchivedAt
	}
	if full.DeletedAt == "" {
		full.DeletedAt = listed.DeletedAt
	}
	return full
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func commandAvailable(name string) bool {
	_, err := os.Stat(filepath.Join("/opt/homebrew/bin", name))
	if err == nil {
		return true
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "outcrawl: "+format+"\n", args...)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `outcrawl mirrors Outline documents into local SQLite and Markdown.

Usage:
  outcrawl [--config path] [--db path] <command> [flags]

Commands:
  init          Write ~/.outcrawl/config.toml
  doctor        Check auth, paths, and API access
  sync          Sync Outline collections/documents into SQLite
  export-md     Render SQLite documents into Markdown files
  search        Search the local archive with SQLite FTS
  status        Show archive counts and last sync time
  version       Print version

Examples:
  outcrawl init
  outcrawl doctor
  outcrawl sync
  outcrawl search "onboarding"`)
}
