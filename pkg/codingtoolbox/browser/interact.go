package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// --- web_click ---

type clickInput struct {
	Selector string `json:"selector"`
}

type clickOutput struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func (b *Browser) clickTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "web_click",
		Description: "Click an element on the current page by CSS selector. After clicking, checks domain trust if the page navigates to a new domain.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string","description":"CSS selector for the element to click"}},"required":["selector"]}`),
		Handler:     b.handleClick,
	}
}

func (b *Browser) handleClick(ctx context.Context, input json.RawMessage) (string, error) {
	var in clickInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("web_click: invalid input: %w", err)
	}

	if in.Selector == "" {
		return "", fmt.Errorf("web_click: selector is required")
	}

	bCtx, err := b.ensureBrowser()
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	if err := chromedp.Run(opCtx,
		chromedp.Click(in.Selector, chromedp.ByQuery),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("web_click: %w", err)
	}

	// Check domain trust after navigation.
	currentURL, title, err := b.pageInfo(opCtx)
	if err != nil {
		return "", err
	}

	if err := b.checkPermission(ctx, currentURL); err != nil {
		return "", err
	}

	out := clickOutput{URL: currentURL, Title: title}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("web_click: marshal: %w", err)
	}

	return string(data), nil
}

// --- web_type ---

type typeInput struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Submit   bool   `json:"submit"`
}

type typeOutput struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func (b *Browser) typeTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "web_type",
		Description: "Type text into an input field on the current page. Optionally submit the form by pressing Enter.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string","description":"CSS selector for the input field"},"text":{"type":"string","description":"Text to type"},"submit":{"type":"boolean","description":"Press Enter after typing (default false)"}},"required":["selector","text"]}`),
		Handler:     b.handleType,
	}
}

func (b *Browser) handleType(ctx context.Context, input json.RawMessage) (string, error) {
	var in typeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("web_type: invalid input: %w", err)
	}

	if in.Selector == "" {
		return "", fmt.Errorf("web_type: selector is required")
	}

	if in.Text == "" {
		return "", fmt.Errorf("web_type: text is required")
	}

	bCtx, err := b.ensureBrowser()
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	actions := []chromedp.Action{
		chromedp.Clear(in.Selector, chromedp.ByQuery),
		chromedp.SendKeys(in.Selector, in.Text, chromedp.ByQuery),
	}

	if in.Submit {
		actions = append(actions,
			chromedp.SendKeys(in.Selector, "\r", chromedp.ByQuery),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)
	}

	if err := chromedp.Run(opCtx, actions...); err != nil {
		return "", fmt.Errorf("web_type: %w", err)
	}

	currentURL, title, err := b.pageInfo(opCtx)
	if err != nil {
		return "", err
	}

	// If a form submit caused navigation, check domain trust on the new URL.
	if in.Submit {
		if err := b.checkPermission(ctx, currentURL); err != nil {
			return "", err
		}
	}

	out := typeOutput{URL: currentURL, Title: title}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("web_type: marshal: %w", err)
	}

	return string(data), nil
}
