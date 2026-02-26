package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type extractInput struct {
	Selector string `json:"selector"`
}

type extractOutput struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

func (b *Browser) extractTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "web_extract",
		Description: "Extract clean text from the current page or a specific element by CSS selector. Scripts, styles, and SVGs are stripped.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string","description":"CSS selector to extract from (default: entire page)"}}}`),
		Handler:     b.handleExtract,
	}
}

func (b *Browser) handleExtract(ctx context.Context, input json.RawMessage) (string, error) {
	var in extractInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("web_extract: invalid input: %w", err)
	}

	bCtx, err := b.ensureBrowser()
	if err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(bCtx, 30*time.Second)
	defer cancel()

	text, err := b.extractText(opCtx, in.Selector)
	if err != nil {
		return "", err
	}

	currentURL, title, err := b.pageInfo(opCtx)
	if err != nil {
		return "", err
	}

	out := extractOutput{
		URL:   currentURL,
		Title: title,
		Text:  text,
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("web_extract: marshal: %w", err)
	}

	return string(data), nil
}
