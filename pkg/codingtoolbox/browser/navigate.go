package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type navigateInput struct {
	URL string `json:"url"`
}

type navigateOutput struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

func (b *Browser) navigateTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "browser_navigate",
		Description: "Navigate to a URL in the Chrome browser and extract the page's clean text content (scripts/styles stripped, 100KB cap). Use this to read documentation pages, API references, external guides, or verify deployed web pages. The page remains loaded for follow-up interactions with browser_click, browser_type, browser_extract, or browser_screenshot. Requires domain trust.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"The URL to navigate to"}},"required":["url"]}`),
		Handler:     b.handleNavigate,
	}
}

func (b *Browser) handleNavigate(ctx context.Context, input json.RawMessage) (string, error) {
	var in navigateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("browser_navigate: invalid input: %w", err)
	}

	if in.URL == "" {
		return "", fmt.Errorf("browser_navigate: url is required")
	}

	if err := b.checkPermission(ctx, in.URL); err != nil {
		return "", err
	}

	bCtx, err := b.ensureBrowser()
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	if err := chromedp.Run(opCtx,
		chromedp.Navigate(in.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("browser_navigate: %w", err)
	}

	text, err := b.extractText(opCtx, "")
	if err != nil {
		return "", err
	}

	currentURL, title, err := b.pageInfo(opCtx)
	if err != nil {
		return "", err
	}

	out := navigateOutput{
		URL:   currentURL,
		Title: title,
		Text:  text,
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("browser_navigate: marshal: %w", err)
	}

	return string(data), nil
}
