package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const sampleArticle = `<!DOCTYPE html>
<html>
<head><title>Real Article</title></head>
<body>
  <nav><a href="/">Home</a> <a href="/about">About</a></nav>
  <div id="content">
    <h1>Why We Wrote Our Own Engines</h1>
    <p>This is the first paragraph of the article. It explains the motivation
    behind building custom inference engines in C and C++.</p>
    <p>Here is a second paragraph with more detail about the design choices
    and trade-offs involved.</p>
    <script>var junk = "this should not appear";</script>
    <style>.junk { color: red; }</style>
  </div>
  <footer>Copyright &copy; 2026 Some Company. All rights reserved.</footer>
</body>
</html>`

func TestFetchTextExtractsArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sampleArticle))
	}))
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	for _, want := range []string{
		"Why We Wrote Our Own Engines",
		"first paragraph of the article",
		"second paragraph with more detail",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text missing %q\nGot:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Home", "About", "Copyright", "junk", "color: red"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("extracted text should not contain %q\nGot:\n%s", unwanted, text)
		}
	}
}

func TestFetchTextPrefersArticleOverBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>
			<p>Boilerplate body text that should not be used.</p>
			<article><p>The real article content lives here.</p></article>
		</body></html>`))
	}))
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "real article content") {
		t.Errorf("expected article content, got: %q", text)
	}
	if strings.Contains(text, "Boilerplate body text") {
		t.Errorf("body text leaked past <article>: %q", text)
	}
}

func TestFetchTextFollowsHTTPRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/article", http.StatusFound)
	})
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>Content after HTTP redirect.</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL+"/start")
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "Content after HTTP redirect") {
		t.Errorf("got %q", text)
	}
}

func TestFetchTextFollowsMetaRefresh(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/stub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head>
			<meta http-equiv="refresh" content="0; url=/real-article">
		</head><body><p>Redirecting&hellip; <a href="/real-article">click here</a></p></body></html>`))
	})
	mux.HandleFunc("/real-article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>The destination article text.</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL+"/stub")
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "destination article text") {
		t.Errorf("meta refresh not followed, got: %q", text)
	}
	if strings.Contains(text, "Redirecting") {
		t.Errorf("stub text leaked: %q", text)
	}
}

func TestFetchTextFollowsJSRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/stub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head>
			<script>window.location.replace("/js-destination");</script>
		</head><body><p>Please enable JavaScript.</p></body></html>`))
	})
	mux.HandleFunc("/js-destination", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>Reached via JS redirect.</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL+"/stub")
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "Reached via JS redirect") {
		t.Errorf("JS redirect not followed, got: %q", text)
	}
}

func TestFetchTextDoesNotFollowRedirectWhenContentPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head>
			<meta http-equiv="refresh" content="0; url=/elsewhere">
		</head><body><p>This page has real content that is long enough to matter
		for summarization purposes, so the refresh tag must be ignored and the
		text returned as-is.</p></body></html>`))
	}))
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "real content") {
		t.Errorf("expected page text returned as-is, got: %q", text)
	}
}

func TestFetchTextRelativeMetaRefreshURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a/stub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head>
			<meta http-equiv="refresh" content="0;url='../b/target'">
		</head><body><p>Redirecting</p></body></html>`))
	})
	mux.HandleFunc("/b/target", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>Relative target reached.</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL+"/a/stub")
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "Relative target reached") {
		t.Errorf("relative meta refresh not followed, got: %q", text)
	}
}

func TestFetchTextPlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Just some plain text content."))
	}))
	defer server.Close()

	text, err := FetchText(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(text, "plain text content") {
		t.Errorf("got %q", text)
	}
}

func TestFetchTextRejectsNonHTTP(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://example.com/x", "data:text/html,hi"} {
		_, err := FetchText(context.Background(), u)
		if err == nil {
			t.Errorf("FetchText(%q) should fail", u)
		}
	}
}

func TestFetchTextRejectsNonHTMLContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer server.Close()

	_, err := FetchText(context.Background(), server.URL)
	if err == nil {
		t.Error("FetchText should fail for image content type")
	}
}

func TestFetchTextEmptyPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("   \n\t "))
	}))
	defer server.Close()

	_, err := FetchText(context.Background(), server.URL)
	if err == nil {
		t.Error("FetchText should fail for empty page")
	}
}

func TestFetchTextHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := FetchText(context.Background(), server.URL)
	if err == nil {
		t.Error("FetchText should fail on HTTP error status")
	}
}

func TestFetchTextRedirectLoop(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><meta http-equiv="refresh" content="0; url=/b"></head><body></body></html>`))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><meta http-equiv="refresh" content="0; url=/a"></head><body></body></html>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := FetchText(context.Background(), server.URL+"/a")
	if err == nil {
		t.Error("FetchText should fail on redirect loop")
	}
}

func TestFetchTextUsesBrowserUserAgent(t *testing.T) {
	var ua string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	if _, err := FetchText(context.Background(), server.URL); err != nil {
		t.Fatalf("FetchText failed: %v", err)
	}
	if !strings.Contains(ua, "Mozilla/5.0") {
		t.Errorf("expected browser-like User-Agent, got %q", ua)
	}
}

func TestStripHTML(t *testing.T) {
	input := `<p>Hello <b>world</b> &amp; friends.</p><p>Second paragraph.</p><script>bad()</script>`
	got := StripHTML(input)
	for _, want := range []string{"Hello world & friends.", "Second paragraph."} {
		if !strings.Contains(got, want) {
			t.Errorf("StripHTML result missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "bad()") {
		t.Errorf("StripHTML kept script content: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("StripHTML should preserve paragraph breaks: %q", got)
	}
}

func TestStripHTMLPlainText(t *testing.T) {
	if got := StripHTML("just text, no tags"); got != "just text, no tags" {
		t.Errorf("StripHTML plain text = %q", got)
	}
}

func TestTextLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   \n\t ", 0},
		{"hello", 5},
		{"hello world", 10},
		{"a  b\tc", 3},
	}
	for _, c := range cases {
		if got := TextLen(c.in); got != c.want {
			t.Errorf("TextLen(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFindMetaRefresh(t *testing.T) {
	cases := []struct {
		html string
		want string
	}{
		{`<meta http-equiv="refresh" content="0; url=https://example.com/x">`, "https://example.com/x"},
		{`<meta content="5;URL='/y'" http-equiv="REFRESH">`, "/y"},
		{`<meta http-equiv="refresh" content="30">`, ""},
		{`<meta http-equiv="refresh" content="0; url=">`, ""},
		{`<meta name="refresh" content="0; url=/x">`, ""},
		{`<meta http-equiv="refresh" content="0; url=https://example.com/a"><meta http-equiv="refresh" content="0; url=https://example.com/b">`, "https://example.com/a"},
	}
	for _, c := range cases {
		doc, err := html.Parse(strings.NewReader(c.html))
		if err != nil {
			t.Fatalf("parse %q: %v", c.html, err)
		}
		if got := findMetaRefresh(doc); got != c.want {
			t.Errorf("findMetaRefresh(%q) = %q, want %q", c.html, got, c.want)
		}
	}
}
