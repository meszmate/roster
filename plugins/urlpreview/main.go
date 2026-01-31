package main

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/meszmate/roster/pkg/plugin"
)

// URLPreviewPlugin shows previews for URLs in messages
type URLPreviewPlugin struct {
	api     plugin.API
	running bool
	unsub   func()
	client  *http.Client
}

// Name returns the plugin name
func (p *URLPreviewPlugin) Name() string {
	return "urlpreview"
}

// Version returns the plugin version
func (p *URLPreviewPlugin) Version() string {
	return "1.0.0"
}

// Description returns a short description
func (p *URLPreviewPlugin) Description() string {
	return "Preview URLs in chat messages"
}

// Init initializes the plugin
func (p *URLPreviewPlugin) Init(ctx context.Context, api plugin.API) error {
	p.api = api
	p.client = &http.Client{
		Timeout: 5 * time.Second,
	}
	return nil
}

// Start starts the plugin
func (p *URLPreviewPlugin) Start() error {
	if p.running {
		return nil
	}

	p.unsub = p.api.OnMessage(func(msg plugin.Message) {
		urls := extractURLs(msg.Body)
		for _, url := range urls {
			go p.previewURL(msg.From, url)
		}
	})

	p.running = true
	return nil
}

// Stop stops the plugin
func (p *URLPreviewPlugin) Stop() error {
	if !p.running {
		return nil
	}

	if p.unsub != nil {
		p.unsub()
		p.unsub = nil
	}

	p.running = false
	return nil
}

// previewURL fetches and displays URL preview
func (p *URLPreviewPlugin) previewURL(from, url string) {
	title, description := fetchURLMeta(p.client, url)
	if title == "" {
		return
	}

	preview := title
	if description != "" {
		preview += ": " + truncate(description, 100)
	}

	// Update status bar with preview
	_ = p.api.AddStatusBarItem("urlpreview", preview)

	// Remove after 10 seconds
	time.Sleep(10 * time.Second)
	_ = p.api.RemoveStatusBarItem("urlpreview")
}

// extractURLs extracts URLs from text
func extractURLs(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	return urlRegex.FindAllString(text, -1)
}

// fetchURLMeta fetches title and description from a URL
func fetchURLMeta(client *http.Client, url string) (string, string) {
	resp, err := client.Get(url)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", ""
	}

	// Read limited body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*100)) // 100KB limit
	if err != nil {
		return "", ""
	}

	html := string(body)

	// Extract title
	title := extractMetaTag(html, "og:title")
	if title == "" {
		title = extractHTMLTitle(html)
	}

	// Extract description
	description := extractMetaTag(html, "og:description")
	if description == "" {
		description = extractMetaTag(html, "description")
	}

	return title, description
}

// extractMetaTag extracts a meta tag value
func extractMetaTag(html, name string) string {
	// Look for <meta property="og:title" content="...">
	// or <meta name="description" content="...">
	patterns := []string{
		`<meta[^>]+property=["']` + name + `["'][^>]+content=["']([^"']+)["']`,
		`<meta[^>]+content=["']([^"']+)["'][^>]+property=["']` + name + `["']`,
		`<meta[^>]+name=["']` + name + `["'][^>]+content=["']([^"']+)["']`,
		`<meta[^>]+content=["']([^"']+)["'][^>]+name=["']` + name + `["']`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// extractHTMLTitle extracts the <title> tag
func extractHTMLTitle(html string) string {
	re := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// truncate truncates a string to max length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func main() {
	// This would use go-plugin to serve the plugin
}
