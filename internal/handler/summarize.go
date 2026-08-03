package handler

import (
	"context"
	"fmt"
	"log"

	"github.com/james/feedlot/internal/model"
	"github.com/james/feedlot/internal/pages"
)

const (
	// minPageText is the smallest extracted page text worth summarizing.
	minPageText = 200
	// minFeedText is the smallest stripped feed content worth summarizing.
	minFeedText = 100
	// fullArticleText is the stripped feed-content length above which the
	// feed is assumed to carry the full article, making a page fetch on
	// background refreshes unnecessary.
	fullArticleText = 1500
)

// articleTextForSummary returns the text an AI summary should be based on:
//
//   - preferPage (explicit "summarize" click): the whole page referenced by
//     the article URL is fetched and extracted first, falling back to the
//     feed-provided content when the page can't be fetched.
//   - otherwise (background refresh): the feed content is used when it
//     already looks like a full article; only when it is missing or a short
//     teaser (Hacker News items carry nothing but a "Comments" link) is the
//     page fetched. This keeps refreshes of full-text feeds fast.
//
// Feed content is HTML and is stripped to plain text before use.
func (h *Handler) articleTextForSummary(ctx context.Context, article *model.Article, preferPage bool) (string, error) {
	feedText := ""
	if article.Content != nil {
		feedText = pages.StripHTML(*article.Content)
	}

	if article.URL != nil && *article.URL != "" {
		if preferPage || pages.TextLen(feedText) < fullArticleText {
			text, err := pages.FetchText(ctx, *article.URL)
			if err == nil {
				if pages.TextLen(text) >= minPageText {
					return text, nil
				}
				log.Printf("summarize article %d: page %s yielded only %d chars", article.ID, *article.URL, pages.TextLen(text))
			} else {
				log.Printf("summarize article %d: fetch page %s: %v", article.ID, *article.URL, err)
			}
		}
	}

	if pages.TextLen(feedText) >= minFeedText {
		return feedText, nil
	}
	return "", fmt.Errorf("no content: feed carries only a teaser and the page could not be fetched")
}
