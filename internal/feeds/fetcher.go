package feeds

import (
	"fmt"
	"strings"

	"github.com/mmcdole/gofeed"

	"github.com/james/feedlot/internal/model"
)

type FetchResult struct {
	Feed     *model.Feed
	Articles []*model.Article
}

func FetchFeed(feedURL string, userID int64) (*FetchResult, error) {
	fp := gofeed.NewParser()
	fp.UserAgent = "Feedlot/1.0 (RSS Reader; +https://github.com/james/feedlot)"

	parsed, err := fp.ParseURL(feedURL)
	if err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", feedURL, err)
	}

	feed := &model.Feed{
		UserID:  userID,
		Title:   parsed.Title,
		FeedURL: feedURL,
	}
	if parsed.Description != "" {
		feed.Description = &parsed.Description
	}
	if parsed.Link != "" {
		feed.SiteURL = &parsed.Link
	}
	if parsed.Image != nil && parsed.Image.URL != "" {
		feed.IconURL = &parsed.Image.URL
	}

	var articles []*model.Article
	seenGUIDs := make(map[string]bool)

	for _, item := range parsed.Items {
		guid := item.GUID
		if guid == "" {
			if item.Link != "" {
				guid = item.Link
			} else if item.Title != "" {
				guid = strings.TrimSpace(item.Title)
			} else {
				continue // skip items with no identifier
			}
		}

		if seenGUIDs[guid] {
			continue
		}
		seenGUIDs[guid] = true

		article := &model.Article{
			GUID: guid,
		}

		if item.Title != "" {
			article.Title = item.Title
		} else {
			article.Title = "(untitled)"
		}

		if item.Link != "" {
			article.URL = &item.Link
		}

		if item.Author != nil && item.Author.Name != "" {
			article.Author = &item.Author.Name
		} else if len(item.Authors) > 0 && item.Authors[0].Name != "" {
			article.Author = &item.Authors[0].Name
		}

		// Prefer full content, fall back to description
		content := ""
		if item.Content != "" {
			content = item.Content
		} else if item.Description != "" {
			content = item.Description
		}
		if content != "" {
			article.Content = &content
		}

		if item.PublishedParsed != nil {
			article.PublishedAt = item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			article.PublishedAt = item.UpdatedParsed
		}

		article.IsRead = false
		articles = append(articles, article)
	}

	result := &FetchResult{
		Feed:     feed,
		Articles: articles,
	}

	return result, nil
}
