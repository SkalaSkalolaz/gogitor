package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"gogitor/internal/textutil"
)

const apiBase = "https://api.github.com"

// Client — минимальный клиент GitHub REST API.
type Client struct {
	token string
	http  *http.Client
	log   *slog.Logger
}

func NewClient(token string, log *slog.Logger) *Client {
	return &Client{
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 30 * time.Second},
		log:   log,
	}
}

// TokenType определяет тип токена GitHub.
func TokenType(token string) string {
	token = strings.TrimSpace(token)
	switch {
	case strings.HasPrefix(token, "ghp_"):
		return "classic PAT"
	case strings.HasPrefix(token, "github_pat_"):
		return "fine-grained PAT"
	case strings.HasPrefix(token, "gho_"):
		return "OAuth"
	case strings.HasPrefix(token, "ghs_"):
		return "GitHub App (server-to-server)"
	case strings.HasPrefix(token, "ghu_"):
		return "GitHub App (user)"
	case token == "":
		return "none"
	default:
		return "unknown"
	}
}

// User — ответ GET /user.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	ID    int64  `json:"id"`
}

// ValidateToken проверяет валидность токена и возвращает пользователя.
func (c *Client) ValidateToken(ctx context.Context) (*User, error) {
	if c.token == "" {
		return nil, fmt.Errorf("github token is empty")
	}
	body, status, err := c.get(ctx, "/user")
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		return nil, fmt.Errorf("github token is invalid or expired (HTTP 401)")
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("github API HTTP %d: %s", status, snippet(body))
	}
	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("cannot parse github user: %w", err)
	}
	return &user, nil
}

// Repo — информация о репозитории.
type Repo struct {
	FullName  string `json:"full_name"`
	CloneURL  string `json:"clone_url"`
	SSHURL    string `json:"ssh_url"`
	Private   bool   `json:"private"`
	DefaultBr string `json:"default_branch"`
}

// RepoInfo возвращает информацию о репозитории.
func (c *Client) RepoInfo(ctx context.Context, owner, repo string) (*Repo, error) {
	path := fmt.Sprintf("/repos/%s/%s", owner, repo)
	body, status, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("repository %s/%s not found (HTTP 404)", owner, repo)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("github API HTTP %d: %s", status, snippet(body))
	}
	var r Repo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("cannot parse repo info: %w", err)
	}
	return &r, nil
}

// CreateRepo создаёт новый репозиторий.
func (c *Client) CreateRepo(ctx context.Context, name string, private bool, description string) (*Repo, error) {
	payload := map[string]any{
		"name":       name,
		"private":    private,
		"auto_init":  false,
	}
	if description != "" {
		payload["description"] = description
	}
	body, status, err := c.post(ctx, "/user/repos", payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnprocessableEntity {
		return nil, fmt.Errorf("repository %q already exists or name is invalid (HTTP 422)", name)
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("github API HTTP %d: %s", status, snippet(body))
	}
	var r Repo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("cannot parse created repo: %w", err)
	}
	return &r, nil
}

// ParseRepoURL извлекает owner и repo из URL.
// Поддерживает:
//   https://github.com/user/repo
//   https://github.com/user/repo.git
//   git@github.com:user/repo.git
func ParseRepoURL(rawURL string) (owner, repo string, err error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimSuffix(rawURL, "/")

	// SSH: git@github.com:user/repo
	if strings.HasPrefix(rawURL, "git@") {
		rest := strings.TrimPrefix(rawURL, "git@")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("cannot parse SSH URL: %s", rawURL)
		}
		segs := strings.Split(strings.Trim(parts[1], "/"), "/")
		if len(segs) < 2 {
			return "", "", fmt.Errorf("cannot parse repo path: %s", rawURL)
		}
		return segs[0], segs[1], nil
	}

	// HTTPS
	trimmed := rawURL
	for _, prefix := range []string{"https://", "http://"} {
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}
	trimmed = strings.TrimPrefix(trimmed, "www.")
	segs := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(segs) < 3 {
		return "", "", fmt.Errorf("cannot parse repo URL: %s", rawURL)
	}
	// segs[0] = github.com, segs[1] = owner, segs[2] = repo
	if !strings.Contains(segs[0], "github") {
		return "", "", fmt.Errorf("not a github URL: %s", rawURL)
	}
	return segs[1], segs[2], nil
}

// ─── HTTP helpers ────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string) ([]byte, int, error) {
	return c.do(ctx, http.MethodGet, apiBase+path, nil)
}

func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	return c.do(ctx, http.MethodPost, apiBase+path, data)
}

func (c *Client) do(ctx context.Context, method, url string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	return textutil.LimitRunes(s, 500, "...")
}

// PullRequest — информация о Pull Request.
type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	State   string `json:"state"`
}

// Issue — информация об Issue.
type Issue struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
}

// CreatePullRequest создаёт Pull Request.
func (c *Client) CreatePullRequest(
	ctx context.Context,
	owner, repo, title, body, head, base string,
) (*PullRequest, error) {
	payload := map[string]any{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	data, status, err := c.post(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnprocessableEntity {
		return nil, fmt.Errorf(
			"cannot create PR (HTTP 422): branch %q may not exist or has no commits ahead of %q",
			head, base,
		)
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("github API HTTP %d: %s", status, snippet(data))
	}
	var pr PullRequest
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("cannot parse PR response: %w", err)
	}
	return &pr, nil
}

// CreateIssue создаёт Issue в репозитории.
func (c *Client) CreateIssue(
	ctx context.Context,
	owner, repo, title, body string,
	labels []string,
) (*Issue, error) {
	payload := map[string]any{
		"title": title,
		"body":  body,
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	path := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)
	data, status, err := c.post(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("github API HTTP %d: %s", status, snippet(data))
	}
	var issue Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		return nil, fmt.Errorf("cannot parse issue response: %w", err)
	}
	return &issue, nil
}

// AddIssueComment добавляет комментарий к Issue или PR.
func (c *Client) AddIssueComment(
	ctx context.Context,
	owner, repo string,
	number int,
	body string,
) error {
	payload := map[string]any{
		"body": body,
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number)
	_, status, err := c.post(ctx, path, payload)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("github API HTTP %d", status)
	}
	return nil
}