package search

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gogitor/internal/textutil"

	"golang.org/x/net/html"
)

type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Result struct {
	Query   string   `json:"query"`
	Content string   `json:"content"`
	Sources []Source `json:"sources"`
}

type Searcher struct {
	http       *http.Client
	log        *slog.Logger
	maxContent int
	urlFilter  func(string) bool
}

func New(log *slog.Logger) *Searcher {
	return &Searcher{
		http:       &http.Client{Timeout: 20 * time.Second},
		log:        log,
		maxContent: 30000,
	}
}

func (s *Searcher) SetURLFilter(filter func(string) bool) {
	s.urlFilter = filter
}

func (s *Searcher) SetMaxContent(max int) {
	if max > 0 {
		s.maxContent = max
	}
}

func (s *Searcher) Search(ctx context.Context, query string) (*Result, error) {
	links, err := s.fetchLinks(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(links) == 0 {
		return nil, fmt.Errorf("no search links found")
	}

	result := &Result{
		Query: query,
	}

	var contentParts []string
	total := 0

	maxTotal := s.maxContent
	if maxTotal <= 0 {
		maxTotal = 30000
	}
	for _, link := range links {
		if total >= maxTotal {
			break
		}

		if s.urlFilter != nil && !s.urlFilter(link.URL) {
			s.log.Debug("search URL filtered out", "url", link.URL)
			continue
		}
		text, err := s.fetchText(ctx, link.URL)
		if err != nil {
			s.log.Warn("search fetch failed", "url", link.URL, "err", err)
			continue
		}

		if strings.TrimSpace(text) == "" {
			continue
		}

		remaining := maxTotal - total

		if len(text) > remaining {
			text = textutil.TruncateStringBytes(text, remaining)
		}

		contentParts = append(contentParts, fmt.Sprintf("SOURCE: %s\nURL: %s\n%s", link.Title, link.URL, text))
		result.Sources = append(result.Sources, link)
		total += len(text)
	}

	if len(contentParts) == 0 {
		return nil, fmt.Errorf("no usable search content found")
	}

	result.Content = strings.Join(contentParts, "\n\n")
	return result, nil
}

func (s *Searcher) fetchLinks(ctx context.Context, query string) ([]Source, error) {
	searchURL := "https://duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Gogitor/1.0)")

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search HTTP %d", resp.StatusCode)
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	var links []Source

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(links) >= 3 {
			return
		}

		if n.Type == html.ElementNode && n.Data == "a" {
			var href string
			var class string

			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
				if attr.Key == "class" {
					class = attr.Val
				}
			}

			if strings.Contains(class, "result__a") && href != "" {
				title := strings.TrimSpace(extractText(n))
				finalURL := href

				if u, err := url.Parse(href); err == nil {
					if q, err := url.ParseQuery(u.RawQuery); err == nil {
						if uddg := q.Get("uddg"); uddg != "" {
							if decoded, err := url.QueryUnescape(uddg); err == nil {
								finalURL = decoded
							}
						}
					}
				}

				if strings.HasPrefix(finalURL, "/") {
					finalURL = "https://duckduckgo.com" + finalURL
				}

				if strings.HasPrefix(finalURL, "http://") || strings.HasPrefix(finalURL, "https://") {
					if !strings.Contains(finalURL, "duckduckgo.com") {
						links = append(links, Source{
							Title: title,
							URL:   finalURL,
						})
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	return links, nil
}

func (s *Searcher) fetchText(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Gogitor/1.0)")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	text := extractText(doc)
	text = normalizeWhitespace(text)

	if len(text) > 10000 {
		text = textutil.TruncateStringBytes(text, 10000)
	}
	return text, nil
}

func extractText(n *html.Node) string {
	var b strings.Builder

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "nav", "header", "footer", "noscript":
				return
			}
		}

		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				b.WriteString(text)
				b.WriteByte(' ')
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(n)

	return b.String()
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
