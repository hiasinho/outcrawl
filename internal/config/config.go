package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const defaultDirName = ".outcrawl"

type Config struct {
	DBPath      string        `toml:"db_path"`
	CacheDir    string        `toml:"cache_dir"`
	MarkdownDir string        `toml:"markdown_dir"`
	Outline     OutlineConfig `toml:"outline"`
}

type OutlineConfig struct {
	BaseURL    string `toml:"base_url"`
	BaseURLEnv string `toml:"base_url_env"`
	TokenEnv   string `toml:"token_env"`
	PageSize   int    `toml:"page_size"`
}

func Default() Config {
	base := filepath.Join(homeDir(), defaultDirName)
	return Config{
		DBPath:      filepath.Join(base, "outcrawl.db"),
		CacheDir:    filepath.Join(base, "cache"),
		MarkdownDir: filepath.Join(base, "pages"),
		Outline: OutlineConfig{
			BaseURL:    "",
			BaseURLEnv: "OUTLINE_BASE_URL",
			TokenEnv:   "OUTLINE_API_TOKEN",
			PageSize:   100,
		},
	}
}

func DefaultPath() string {
	return filepath.Join(homeDir(), defaultDirName, "config.toml")
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath()
	}
	path = ExpandPath(path)
	cfg := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg.Resolve()
		}
		return Config{}, err
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg.Resolve()
}

func WriteStarter(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath()
	}
	path = ExpandPath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	cfg, err := Default().Resolve()
	if err != nil {
		return "", err
	}
	b, err := toml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o600)
}

func (c Config) Resolve() (Config, error) {
	c.DBPath = ExpandPath(c.DBPath)
	c.CacheDir = ExpandPath(c.CacheDir)
	c.MarkdownDir = ExpandPath(c.MarkdownDir)
	if strings.TrimSpace(c.Outline.TokenEnv) == "" {
		c.Outline.TokenEnv = "OUTLINE_API_TOKEN"
	}
	if strings.TrimSpace(c.Outline.BaseURLEnv) == "" {
		c.Outline.BaseURLEnv = "OUTLINE_BASE_URL"
	}
	if c.Outline.PageSize <= 0 || c.Outline.PageSize > 100 {
		c.Outline.PageSize = 100
	}
	c.Outline.BaseURL = strings.TrimRight(strings.TrimSpace(c.Outline.BaseURL), "/")
	return c, nil
}

func (c Config) TokenFromEnv() string {
	return strings.TrimSpace(os.Getenv(c.Outline.TokenEnv))
}

func (c Config) BaseURLFromEnv() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(c.Outline.BaseURLEnv)), "/")
}

func EnsureDirs(c Config) error {
	for _, dir := range []string{filepath.Dir(c.DBPath), c.CacheDir, c.MarkdownDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}
