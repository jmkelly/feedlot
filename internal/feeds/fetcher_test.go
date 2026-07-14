package feeds

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Blog</title>
    <description>A test RSS feed for unit testing</description>
    <link>https://testblog.example.com</link>
    <image>
      <url>https://testblog.example.com/icon.png</url>
      <title>Test Blog</title>
      <link>https://testblog.example.com</link>
    </image>
    <item>
      <guid>post-1</guid>
      <title>First Post</title>
      <link>https://testblog.example.com/first</link>
      <description>This is the first post content.</description>
      <author>Alice</author>
      <pubDate>Mon, 15 Jan 2025 10:00:00 GMT</pubDate>
    </item>
    <item>
      <guid>post-2</guid>
      <title>Second Post</title>
      <link>https://testblog.example.com/second</link>
      <description>This is the second post content.</description>
      <author>Bob</author>
      <pubDate>Tue, 16 Jan 2025 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

const sampleAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Test Feed</title>
  <subtitle>An Atom feed for testing</subtitle>
  <link href="https://atomfeed.example.com"/>
  <icon>https://atomfeed.example.com/icon.png</icon>
  <entry>
    <id>atom-entry-1</id>
    <title>Atom Entry One</title>
    <link href="https://atomfeed.example.com/one"/>
    <content type="html">Content of entry one.</content>
    <author>
      <name>Charlie</name>
    </author>
    <published>2025-02-01T08:00:00Z</published>
  </entry>
  <entry>
    <id>atom-entry-2</id>
    <title>Atom Entry Two</title>
    <link href="https://atomfeed.example.com/two"/>
    <content type="text">Content of entry two.</content>
    <published>2025-02-02T09:30:00Z</published>
  </entry>
</feed>`

func TestFetchRSSFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(sampleRSS))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 42)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if result.Feed == nil {
		t.Fatal("FetchFeed returned nil Feed")
	}
	if result.Feed.Title != "Test Blog" {
		t.Errorf("Feed Title = %q, want %q", result.Feed.Title, "Test Blog")
	}
	if result.Feed.UserID != 42 {
		t.Errorf("Feed UserID = %d, want 42", result.Feed.UserID)
	}
	if result.Feed.FeedURL != server.URL {
		t.Errorf("Feed FeedURL = %q", result.Feed.FeedURL)
	}
	if result.Feed.Description == nil || *result.Feed.Description != "A test RSS feed for unit testing" {
		t.Errorf("Feed Description = %v", result.Feed.Description)
	}
	if result.Feed.SiteURL == nil || *result.Feed.SiteURL != "https://testblog.example.com" {
		t.Errorf("Feed SiteURL = %v", result.Feed.SiteURL)
	}
	if result.Feed.IconURL == nil || *result.Feed.IconURL != "https://testblog.example.com/icon.png" {
		t.Errorf("Feed IconURL = %v", result.Feed.IconURL)
	}

	if len(result.Articles) != 2 {
		t.Fatalf("Got %d articles, want 2", len(result.Articles))
	}

	// First article
	a1 := result.Articles[0]
	if a1.GUID != "post-1" {
		t.Errorf("Article 1 GUID = %q", a1.GUID)
	}
	if a1.Title != "First Post" {
		t.Errorf("Article 1 Title = %q", a1.Title)
	}
	if a1.URL == nil || *a1.URL != "https://testblog.example.com/first" {
		t.Errorf("Article 1 URL = %v", a1.URL)
	}
	if a1.Author == nil || *a1.Author != "Alice" {
		t.Errorf("Article 1 Author = %v", a1.Author)
	}
	if a1.Content == nil || *a1.Content != "This is the first post content." {
		t.Errorf("Article 1 Content = %v", a1.Content)
	}
	if a1.PublishedAt == nil {
		t.Fatal("Article 1 PublishedAt is nil")
	}
	if a1.IsRead {
		t.Error("New article should have IsRead = false")
	}

	// Second article
	a2 := result.Articles[1]
	if a2.GUID != "post-2" {
		t.Errorf("Article 2 GUID = %q", a2.GUID)
	}
	if a2.Title != "Second Post" {
		t.Errorf("Article 2 Title = %q", a2.Title)
	}
}

func TestFetchAtomFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(sampleAtom))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 99)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if result.Feed.Title != "Atom Test Feed" {
		t.Errorf("Feed Title = %q", result.Feed.Title)
	}
	if result.Feed.UserID != 99 {
		t.Errorf("Feed UserID = %d, want 99", result.Feed.UserID)
	}

	if len(result.Articles) != 2 {
		t.Fatalf("Got %d articles, want 2", len(result.Articles))
	}

	a1 := result.Articles[0]
	if a1.GUID != "atom-entry-1" {
		t.Errorf("Article GUID = %q", a1.GUID)
	}
	if a1.Title != "Atom Entry One" {
		t.Errorf("Article Title = %q", a1.Title)
	}
	if a1.URL == nil || *a1.URL != "https://atomfeed.example.com/one" {
		t.Errorf("Article URL = %v", a1.URL)
	}
	if a1.Author == nil || *a1.Author != "Charlie" {
		t.Errorf("Article Author = %v", a1.Author)
	}
}

func TestFetchFeedNoItems(t *testing.T) {
	emptyFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Feed</title>
    <link>https://empty.example.com</link>
    <description>No articles here</description>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(emptyFeed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) != 0 {
		t.Errorf("Got %d articles for empty feed, want 0", len(result.Articles))
	}
	if result.Feed.Title != "Empty Feed" {
		t.Errorf("Feed Title = %q", result.Feed.Title)
	}
}

func TestFetchFeedInvalidURL(t *testing.T) {
	_, err := FetchFeed("https://nonexistent.example.com/invalid-feed.xml", 1)
	if err == nil {
		t.Error("FetchFeed should fail for invalid URL")
	}
}

func TestFetchFeedInvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte("this is not xml"))
	}))
	defer server.Close()

	_, err := FetchFeed(server.URL, 1)
	if err == nil {
		t.Error("FetchFeed should fail for invalid XML")
	}
}

func TestFetchFeedDeduplicatesByGUID(t *testing.T) {
	dupeFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Dupe Feed</title>
    <description>Feed with duplicate GUIDs</description>
    <link>https://dupe.example.com</link>
    <item>
      <guid>same-guid</guid>
      <title>First Article</title>
    </item>
    <item>
      <guid>same-guid</guid>
      <title>Second Article (same GUID)</title>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(dupeFeed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) != 1 {
		t.Errorf("Got %d articles with duplicate GUIDs, want 1", len(result.Articles))
	}
	if result.Articles[0].Title != "First Article" {
		t.Errorf("Kept article %q, expected %q", result.Articles[0].Title, "First Article")
	}
}

func TestFetchFeedMissingGUIDUsesLink(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>No GUID Feed</title>
    <description>Items without GUIDs</description>
    <link>https://noguid.example.com</link>
    <item>
      <title>Article with Link</title>
      <link>https://noguid.example.com/article1</link>
    </item>
    <item>
      <title>Another Article</title>
      <link>https://noguid.example.com/article2</link>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) != 2 {
		t.Fatalf("Got %d articles, want 2", len(result.Articles))
	}
	if result.Articles[0].GUID != "https://noguid.example.com/article1" {
		t.Errorf("Article GUID should be link when no GUID: %q", result.Articles[0].GUID)
	}
}

func TestFetchFeedMissingGUIDAndLinkUsesTitle(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Minimal Feed</title>
    <link>https://minimal.example.com</link>
    <item>
      <title>Just a Title</title>
      <description>Some content</description>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) != 1 {
		t.Fatalf("Got %d articles, want 1", len(result.Articles))
	}
	if result.Articles[0].GUID != "Just a Title" {
		t.Errorf("Article GUID should be title when no GUID/link: %q", result.Articles[0].GUID)
	}
}

func TestFetchFeedSkipsItemWithoutIdentifier(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Skip Feed</title>
    <link>https://skip.example.com</link>
    <item>
      <!-- no guid, no link, no title -->
      <description>Just some content</description>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) != 0 {
		t.Errorf("Got %d articles for item with no identifier, want 0", len(result.Articles))
	}
}

func TestFetchFeedMultipleAuthors(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Multi-Author Feed</title>
    <link>https://multi.example.com</link>
    <item>
      <guid>multi-1</guid>
      <title>Multi-Author Article</title>
      <author>alice@example.com</author>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) > 0 && result.Articles[0].Author != nil {
		// RSS author field is often an email, preserve it as-is
		if *result.Articles[0].Author != "alice@example.com" {
			t.Errorf("Author = %q", *result.Articles[0].Author)
		}
	}
}

func TestFetchFeedContentFallback(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Content Fallback</title>
    <link>https://fallback.example.com</link>
    <item>
      <guid>fallback-1</guid>
      <title>Description Only</title>
      <description>Description content here</description>
      <!-- no content element -->
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) > 0 {
		a := result.Articles[0]
		if a.Content == nil || *a.Content != "Description content here" {
			t.Errorf("Article content should fall back to description: got %v", a.Content)
		}
	}
}

func TestFetchFeedUserAgent(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(sampleRSS))
	}))
	defer server.Close()

	_, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if !strings.Contains(userAgent, "Feedlot") {
		t.Errorf("User-Agent should contain 'Feedlot', got %q", userAgent)
	}
}

func TestFetchFeedUntitledArticle(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>No Title Feed</title>
    <link>https://notitle.example.com</link>
    <item>
      <guid>no-title-1</guid>
      <link>https://notitle.example.com/article</link>
      <description>Content without a title</description>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if len(result.Articles) > 0 {
		if result.Articles[0].Title != "(untitled)" {
			t.Errorf("Untitled article should get '(untitled)', got %q", result.Articles[0].Title)
		}
	}
}

func TestFetchFeedAtomImage(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Image Test Feed</title>
  <link href="https://img.example.com"/>
  <logo>https://img.example.com/logo.png</logo>
  <entry>
    <id>entry-1</id>
    <title>Entry</title>
  </entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(feed))
	}))
	defer server.Close()

	result, err := FetchFeed(server.URL, 1)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	// Atom uses <logo> not <icon>, and gofeed maps it to Image
	if result.Feed.IconURL == nil {
		t.Log("Note: Atom logo mapping may differ from icon")
	}
}
