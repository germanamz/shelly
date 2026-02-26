package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type screenshotInput struct {
	Selector string `json:"selector"`
	FullPage bool   `json:"full_page"`
}

type screenshotOutput struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Base64 string `json:"base64"`
}

func (b *Browser) screenshotTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "web_screenshot",
		Description: "Take a PNG screenshot of the current page viewport, full page, or a specific element. Returns base64-encoded image.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string","description":"CSS selector for element screenshot (default: viewport)"},"full_page":{"type":"boolean","description":"Capture full scrollable page (default false)"}}}`),
		Handler:     b.handleScreenshot,
	}
}

func (b *Browser) handleScreenshot(ctx context.Context, input json.RawMessage) (string, error) {
	var in screenshotInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("web_screenshot: invalid input: %w", err)
	}

	bCtx, err := b.ensureBrowser()
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	var buf []byte

	switch {
	case in.Selector != "":
		if err := chromedp.Run(opCtx,
			chromedp.Screenshot(in.Selector, &buf, chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("web_screenshot: %w", err)
		}
	case in.FullPage:
		if err := chromedp.Run(opCtx,
			chromedp.FullScreenshot(&buf, 90),
		); err != nil {
			return "", fmt.Errorf("web_screenshot: %w", err)
		}
	default:
		if err := chromedp.Run(opCtx,
			chromedp.CaptureScreenshot(&buf),
		); err != nil {
			return "", fmt.Errorf("web_screenshot: %w", err)
		}
	}

	currentURL, title, err := b.pageInfo(opCtx)
	if err != nil {
		return "", err
	}

	out := screenshotOutput{
		URL:    currentURL,
		Title:  title,
		Base64: base64.StdEncoding.EncodeToString(buf),
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("web_screenshot: marshal: %w", err)
	}

	return string(data), nil
}
