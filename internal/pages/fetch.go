// Package pages fetches article pages and extracts their readable text.
//
// Many feeds (notably Hacker News) carry only a link and a short teaser
// in each item, so summarizing the feed content alone yields useless
// results. This package retrieves the page the feed item points at —
// following HTTP redirects as well as meta-refresh and JavaScript
// redirects some sites use — and reduces it to plain text suitable for
// an AI summarizer.
package pages

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	// fetchTimeout bounds a single page fetch.
	fetchTimeout = 30 * time.Second
	// maxBodyBytes caps how much of a page we read (5 MiB decoded).
	maxBodyBytes = 5 << 20
	// maxRedirectHops bounds how many meta-refresh / JS redirect hops we follow.
	maxRedirectHops = 5
	// redirectThreshold is the maximum extracted-text length a page may have
	// before we still treat it as a pure redirector page and chase its
	// meta-refresh / JS redirect. Pages with more text are real content.
	redirectThreshold = 100

	// userAgent is a browser-like UA: several sites serve bot-blocking pages
	// (or redirect stubs) to unknown user agents.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Feedlot/1.0"
)

// stripTags are elements whose contents are never article text.
var stripTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"nav": true, "header": true, "footer": true, "aside": true,
	"form": true, "svg": true, "iframe": true, "canvas": true,
	"audio": true, "video": true, "button": true, "input": true,
	"select": true, "textarea": true,
}

// blockTags get a newline after them so paragraph structure survives
// text extraction.
var blockTags = map[string]bool{
	"address": true, "article": true, "blockquote": true, "br": true,
	"div": true, "dl": true, "figcaption": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"hr": true, "li": true, "main": true, "ol": true, "p": true,
	"pre": true, "section": true, "table": true, "tr": true, "ul": true,
}

var jsRedirectRe = regexp.MustCompile(
	`(?:window\.)?location\.(?:replace|assign)\(\s*["']([^"']+)["']\s*\)|` +
		`(?:window\.)?location\.href\s*=\s*["']([^"']+)["']|` +
		`(?:window\.)?location\s*=\s*["']([^"']+)["']`)

// FetchText retrieves the page at rawURL and returns its main text
// content, reduced to plain text with paragraphs separated by newlines.
//
// HTTP redirects are followed by the client automatically; meta-refresh
// and JavaScript redirects (common on link-shortener and "are you a
// robot" stubs) are chased manually for a bounded number of hops, but
// only when the page carries essentially no real content of its own.
func FetchText(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}

	client := &http.Client{Timeout: fetchTimeout}

	cur := rawURL
	seen := map[string]bool{}
	for hop := 0; hop <= maxRedirectHops; hop++ {
		if seen[cur] {
			return "", fmt.Errorf("redirect loop at %s", cur)
		}
		seen[cur] = true

		result, err := fetchOnce(ctx, client, cur)
		if err != nil {
			return "", err
		}
		if result.next == "" {
			return result.text, nil
		}
		cur = result.next
	}
	return "", fmt.Errorf("too many redirects (max %d)", maxRedirectHops)
}

type fetchResult struct {
	text string
	next string // resolved meta-refresh / JS redirect target, empty when none
}

func fetchOnce(ctx context.Context, client *http.Client, pageURL string) (fetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return fetchResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return fetchResult{}, fmt.Errorf("get %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fetchResult{}, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, pageURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fetchResult{}, fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// Decode non-UTF-8 charsets (latin-1, shift_jis, ...) to UTF-8.
	enc, _, _ := charset.DetermineEncoding(body, contentType)
	if decoded, derr := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(body))); derr == nil {
		body = decoded
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		return fetchResult{}, fmt.Errorf("empty page at %s", pageURL)
	}

	lowerCT := strings.ToLower(contentType)
	switch {
	case strings.Contains(lowerCT, "html"), contentType == "":
		return processHTML(pageURL, body)
	case strings.HasPrefix(lowerCT, "text/plain"):
		return fetchResult{text: collapseWhitespace(string(body))}, nil
	default:
		// Some redirector stubs serve application/octet-stream; sniff for HTML.
		trimmed := strings.TrimSpace(string(body))
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") {
			return processHTML(pageURL, body)
		}
		return fetchResult{}, fmt.Errorf("unsupported content type %q from %s", contentType, pageURL)
	}
}

// processHTML extracts text from an HTML document. When the page turns
// out to be a bare redirector stub (meta refresh or JS location change
// and no real content), it returns the resolved target URL instead.
func processHTML(pageURL string, body []byte) (fetchResult, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return fetchResult{}, fmt.Errorf("parse html: %w", err)
	}

	text := extractText(doc)

	if TextLen(text) < redirectThreshold {
		if target := findMetaRefresh(doc); target != "" {
			if abs, ok := resolveRedirect(pageURL, target); ok && abs != pageURL {
				return fetchResult{next: abs}, nil
			}
		}
		if target := findJSRedirect(doc); target != "" {
			if abs, ok := resolveRedirect(pageURL, target); ok && abs != pageURL {
				return fetchResult{next: abs}, nil
			}
		}
	}

	return fetchResult{text: text}, nil
}

// extractText returns the readable text of an HTML document, preferring
// the <article> element, then <main>, then <body>.
func extractText(doc *html.Node) string {
	root := doc
	if n := findFirst(doc, "article"); n != nil {
		root = n
	} else if n := findFirst(doc, "main"); n != nil {
		root = n
	} else if n := findFirst(doc, "body"); n != nil {
		root = n
	}

	var sb strings.Builder
	walkText(root, &sb)
	return collapseWhitespace(sb.String())
}

func walkText(n *html.Node, sb *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		sb.WriteString(n.Data)
		return
	case html.ElementNode:
		name := strings.ToLower(n.Data)
		if stripTags[name] {
			return
		}
		if blockTags[name] {
			ensureNewline(sb)
		}
	case html.CommentNode, html.DoctypeNode:
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkText(c, sb)
	}
	if n.Type == html.ElementNode && blockTags[strings.ToLower(n.Data)] {
		ensureNewline(sb)
	}
}

func ensureNewline(sb *strings.Builder) {
	if sb.Len() > 0 && sb.String()[sb.Len()-1] != '\n' {
		sb.WriteByte('\n')
	}
}

func findFirst(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && strings.ToLower(n.Data) == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// findMetaRefresh returns the URL from a <meta http-equiv="refresh"
// content="0; url=..."> element, or "" when absent.
func findMetaRefresh(doc *html.Node) string {
	var found string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "meta" {
			var httpEquiv, content string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "http-equiv":
					httpEquiv = a.Val
				case "content":
					content = a.Val
				}
			}
			if strings.EqualFold(httpEquiv, "refresh") && content != "" {
				lower := strings.ToLower(content)
				idx := strings.Index(lower, "url=")
				if idx >= 0 {
					target := strings.Trim(content[idx+len("url="):], " \t\r\n\"'")
					if target != "" {
						found = target
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

// findJSRedirect returns the first URL assigned to window.location /
// location.href / location.replace in any script block, or "".
func findJSRedirect(doc *html.Node) string {
	var found string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "script" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.TextNode {
					continue
				}
				if m := jsRedirectRe.FindStringSubmatch(c.Data); m != nil {
					for _, g := range m[1:] {
						if g != "" {
							found = g
							return
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

// resolveRedirect resolves a (possibly relative) redirect target against
// pageURL, accepting only http/https targets.
func resolveRedirect(pageURL, target string) (string, bool) {
	u, err := url.Parse(target)
	if err != nil {
		return "", false
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return "", false
	}
	abs := base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return "", false
	}
	return abs.String(), true
}

// StripHTML reduces HTML (typically feed item content) to plain text,
// preserving paragraph breaks.
func StripHTML(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return collapseWhitespace(s)
	}
	return extractText(doc)
}

// TextLen counts non-whitespace characters.
func TextLen(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// collapseWhitespace trims each line and drops blank lines.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			result = append(result, t)
		}
	}
	return strings.Join(result, "\n")
}
