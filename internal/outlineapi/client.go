package outlineapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/hiasinho/outcrawl/internal/config"
)

const defaultTimeout = 30 * time.Second

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type AuthSource struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"-"`
	Source  string `json:"source"`
}

type Collection struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Color       string          `json:"color"`
	Private     bool            `json:"private"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Raw         json.RawMessage `json:"-"`
}

type Document struct {
	ID               string          `json:"id"`
	CollectionID     string          `json:"collectionId"`
	ParentDocumentID string          `json:"parentDocumentId"`
	Title            string          `json:"title"`
	Text             string          `json:"text"`
	URL              string          `json:"url"`
	URLID            string          `json:"urlId"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
	PublishedAt      string          `json:"publishedAt"`
	ArchivedAt       string          `json:"archivedAt"`
	DeletedAt        string          `json:"deletedAt"`
	Raw              json.RawMessage `json:"-"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func ResolveAuth(ctx context.Context, cfg config.Config) (AuthSource, error) {
	token := cfg.TokenFromEnv()
	baseURL := cfg.BaseURLFromEnv()
	if baseURL == "" {
		baseURL = cfg.Outline.BaseURL
	}
	if token != "" {
		return AuthSource{BaseURL: normalizeBaseURL(baseURL), Token: token, Source: "env"}, nil
	}

	olToken, tokenErr := runOL(ctx, "auth", "token", "view")
	if tokenErr == nil {
		token = strings.TrimSpace(olToken)
	}
	if baseURL == "" {
		if olCurrent, err := runOL(ctx, "account", "current", "--json"); err == nil {
			baseURL = parseOLBaseURL([]byte(olCurrent))
		}
	}
	if token == "" {
		if tokenErr != nil {
			return AuthSource{}, fmt.Errorf("missing Outline token: set %s or run ol auth login/token (%v)", cfg.Outline.TokenEnv, tokenErr)
		}
		return AuthSource{}, fmt.Errorf("missing Outline token: set %s or run ol auth login/token", cfg.Outline.TokenEnv)
	}
	if baseURL == "" {
		return AuthSource{}, fmt.Errorf("missing Outline base URL: set %s, configure outline.base_url, or select an ol account", cfg.Outline.BaseURLEnv)
	}
	return AuthSource{BaseURL: normalizeBaseURL(baseURL), Token: token, Source: "ol"}, nil
}

func New(auth AuthSource) *Client {
	return &Client{BaseURL: normalizeBaseURL(auth.BaseURL), Token: auth.Token, HTTP: &http.Client{Timeout: defaultTimeout}}
}

func (c *Client) AuthInfo(ctx context.Context) (User, error) {
	var out struct {
		Data User `json:"data"`
	}
	if err := c.post(ctx, "auth.info", map[string]any{}, &out); err != nil {
		return User{}, err
	}
	return out.Data, nil
}

func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	var all []Collection
	offset := 0
	limit := 100
	for {
		var out struct {
			Data       []json.RawMessage `json:"data"`
			Pagination pagination        `json:"pagination"`
		}
		if err := c.post(ctx, "collections.list", map[string]any{"limit": limit, "offset": offset}, &out); err != nil {
			return nil, err
		}
		for _, raw := range out.Data {
			var col Collection
			if err := json.Unmarshal(raw, &col); err != nil {
				return nil, err
			}
			col.Raw = raw
			all = append(all, col)
		}
		if !hasNext(out.Pagination, len(out.Data), limit) {
			break
		}
		offset += limit
	}
	return all, nil
}

func (c *Client) ListDocuments(ctx context.Context, pageSize int) ([]Document, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 100
	}
	var all []Document
	offset := 0
	for {
		var out struct {
			Data       []json.RawMessage `json:"data"`
			Pagination pagination        `json:"pagination"`
		}
		payload := map[string]any{"limit": pageSize, "offset": offset, "sort": "updatedAt", "direction": "DESC"}
		if err := c.post(ctx, "documents.list", payload, &out); err != nil {
			return nil, err
		}
		for _, raw := range out.Data {
			var doc Document
			if err := json.Unmarshal(raw, &doc); err != nil {
				return nil, err
			}
			doc.Raw = raw
			all = append(all, doc)
		}
		if !hasNext(out.Pagination, len(out.Data), pageSize) {
			break
		}
		offset += pageSize
	}
	return all, nil
}

func (c *Client) GetDocument(ctx context.Context, id string) (Document, error) {
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.post(ctx, "documents.info", map[string]any{"id": id}, &out); err != nil {
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(out.Data, &doc); err != nil {
		return Document{}, err
	}
	doc.Raw = out.Data
	return doc, nil
}

type pagination struct {
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	NextPath string `json:"nextPath"`
	Total    int    `json:"total"`
}

func hasNext(p pagination, got int, limit int) bool {
	if got == 0 {
		return false
	}
	if p.Total > 0 {
		return p.Offset+got < p.Total
	}
	return strings.TrimSpace(p.NextPath) != ""
}

func (c *Client) post(ctx context.Context, method string, payload any, target any) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("missing Outline base URL")
	}
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("missing Outline token")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/api/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("outline %s: HTTP %d: %s", method, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode outline %s: %w", method, err)
	}
	return nil
}

func runOL(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmdArgs := append([]string{"--no-spinner"}, args...)
	cmd := exec.CommandContext(ctx, "ol", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseOLBaseURL(b []byte) string {
	var current struct {
		Source  string `json:"source"`
		Account struct {
			BaseURL string `json:"baseUrl"`
		} `json:"account"`
	}
	if err := json.Unmarshal(b, &current); err == nil && current.Account.BaseURL != "" {
		return strings.TrimRight(current.Account.BaseURL, "/")
	}
	return ""
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	return strings.TrimSuffix(raw, "/api")
}
