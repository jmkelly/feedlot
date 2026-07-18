package feeds

import (
	"log"
	"time"

	"github.com/james/feedlot/internal/model"
)

// FeedStore defines the database methods the refresher needs.
type FeedStore interface {
	GetAllFeeds() ([]model.Feed, error)
	CreateArticle(a *model.Article) (*model.Article, error)
	UpdateFeedLastFetched(id int64, t time.Time) error
}

// Refresher periodically fetches all feeds in the background.
type Refresher struct {
	store    FeedStore
	interval time.Duration
	stopCh   chan struct{}
}

// NewRefresher creates a new background feed refresher.
func NewRefresher(store FeedStore, interval time.Duration) *Refresher {
	return &Refresher{
		store:    store,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background refresh loop. It does an initial refresh
// shortly after startup, then repeats at the configured interval.
func (r *Refresher) Start() {
	go r.loop()
}

// Stop signals the refresh loop to shut down.
func (r *Refresher) Stop() {
	close(r.stopCh)
}

func (r *Refresher) loop() {
	// Brief pause at startup so the server can settle
	time.Sleep(10 * time.Second)
	r.refreshAll()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.refreshAll()
		case <-r.stopCh:
			log.Println("refresher: stopped")
			return
		}
	}
}

func (r *Refresher) refreshAll() {
	feeds, err := r.store.GetAllFeeds()
	if err != nil {
		log.Printf("refresher: get all feeds: %v", err)
		return
	}

	log.Printf("refresher: refreshing %d feeds", len(feeds))

	for i := range feeds {
		r.refreshFeed(&feeds[i])
	}
}

func (r *Refresher) refreshFeed(feed *model.Feed) {
	result, err := FetchFeed(feed.FeedURL, feed.UserID)
	if err != nil {
		log.Printf("refresher: fetch feed %q (id=%d): %v", feed.FeedURL, feed.ID, err)
		return
	}

	now := time.Now()
	newCount := 0

	for _, article := range result.Articles {
		article.FeedID = feed.ID
		if _, err := r.store.CreateArticle(article); err != nil {
			// Duplicate GUID — skip silently
			continue
		}
		newCount++
	}

	if err := r.store.UpdateFeedLastFetched(feed.ID, now); err != nil {
		log.Printf("refresher: update last_fetched for feed %d: %v", feed.ID, err)
	}

	if newCount > 0 {
		log.Printf("refresher: feed %q (id=%d): %d new articles", feed.Title, feed.ID, newCount)
	}
}
