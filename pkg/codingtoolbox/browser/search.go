package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type searchInput struct {
	Query string `json:"query"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (b *Browser) searchTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "browser_search",
		Description: "Search the web using a Chrome browser via DuckDuckGo. Use this to find documentation, API references, library guides, error solutions, or any external information. Returns an array of {title, url, snippet}. Pair with browser_navigate to read full page content from the results.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"}},"required":["query"]}`),
		Handler:     b.handleSearch,
	}
}

func (b *Browser) handleSearch(ctx context.Context, input json.RawMessage) (string, error) {
	var in searchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("browser_search: invalid input: %w", err)
	}

	if in.Query == "" {
		return "", fmt.Errorf("browser_search: query is required")
	}

	bCtx, err := b.ensureBrowser(ctx)
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	// DuckDuckGo is internal search infrastructure — no user permission required.
	// The agent cannot control which domain DDG loads; user-chosen URLs go through
	// web_navigate which does require domain trust.
	ddgURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(in.Query)

	var results []searchResult
	err = chromedp.Run(opCtx,
		chromedp.Navigate(ddgURL),
		chromedp.WaitVisible(`.result`, chromedp.ByQuery),
		chromedp.Evaluate(`
			(function() {
				var results = [];
				var items = document.querySelectorAll(".result");
				for (var i = 0; i < items.length && i < 30; i++) {
					var a = items[i].querySelector(".result__title a, .result__a");
					var s = items[i].querySelector(".result__snippet");
					if (a) {
						results.push({
							title: (a.textContent || "").trim(),
							url: a.href || "",
							snippet: s ? (s.textContent || "").trim() : ""
						});
					}
				}
				return results;
			})()
		`, &results),
	)
	if err != nil {
		return "", fmt.Errorf("browser_search: %w", err)
	}

	data, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("browser_search: marshal: %w", err)
	}

	return string(data), nil
}
