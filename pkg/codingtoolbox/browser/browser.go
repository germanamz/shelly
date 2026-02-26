// Package browser provides tools that give agents browser automation via
// headless Chrome. Each domain is gated by the shared permission store.
// The Chrome process is started lazily on first tool use and runs in
// incognito mode to avoid leaking cookies or user profiles.
package browser

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// Option configures Browser behaviour.
type Option func(*Browser)

// WithHeadless enables headless Chrome mode (no visible window).
func WithHeadless() Option {
	return func(b *Browser) { b.headless = true }
}

// maxContentBytes is the maximum extracted text size (100KB).
const maxContentBytes = 100 * 1024

// Browser provides browser automation tools with permission gating.
type Browser struct {
	store *permissions.Store
	ask   AskFunc

	headless  bool
	parentCtx context.Context

	mu          sync.Mutex
	started     bool
	browserCtx  context.Context
	browserDone context.CancelFunc
	allocDone   context.CancelFunc
}

// New creates a Browser that checks the given permissions store for trusted
// domains and prompts the user via askFn when a domain is not yet trusted.
// The parentCtx is used as the root context for the Chrome process; cancelling
// it will tear down Chrome.
func New(parentCtx context.Context, store *permissions.Store, askFn AskFunc, opts ...Option) *Browser {
	b := &Browser{
		store:     store,
		ask:       askFn,
		parentCtx: parentCtx,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Tools returns a ToolBox containing all browser tools.
func (b *Browser) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		b.searchTool(),
		b.navigateTool(),
		b.clickTool(),
		b.typeTool(),
		b.extractTool(),
		b.screenshotTool(),
	)
	return tb
}

// Close shuts down the Chrome process if it was started.
func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return
	}

	b.browserDone()
	b.allocDone()
	b.browserDone = nil
	b.allocDone = nil
	b.browserCtx = nil
	b.started = false
}

// ensureBrowser lazily starts the Chrome process on first call.
func (b *Browser) ensureBrowser() (context.Context, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return b.browserCtx, nil
	}

	// Build allocator options: start from defaults, then adjust for headed/headless.
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if !b.headless {
		// Override headless flag so Chrome opens a visible window.
		opts = append(opts, chromedp.Flag("headless", false))
	}
	// Always incognito + disable-gpu for stability.
	opts = append(opts,
		chromedp.Flag("incognito", true),
		chromedp.Flag("disable-gpu", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(b.parentCtx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	// Force Chrome to start by running a noop.
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return nil, fmt.Errorf("browser: start chrome: %w", err)
	}

	b.browserCtx = browserCtx
	b.browserDone = browserCancel
	b.allocDone = allocCancel
	b.started = true

	return b.browserCtx, nil
}

// checkPermission checks if a domain is trusted, prompting the user if not.
func (b *Browser) checkPermission(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browser: invalid URL: %w", err)
	}

	domain := parsed.Hostname()
	if domain == "" {
		return fmt.Errorf("browser: could not extract domain from URL")
	}

	if b.store.IsDomainTrusted(domain) {
		return nil
	}

	resp, err := b.ask(ctx, fmt.Sprintf("Allow browser access to %s?", domain), []string{"yes", "trust", "no"})
	if err != nil {
		return fmt.Errorf("browser: ask permission: %w", err)
	}

	switch strings.ToLower(resp) {
	case "trust":
		return b.store.TrustDomain(domain)
	case "yes":
		return nil
	default:
		return fmt.Errorf("browser: access denied to %s", domain)
	}
}

// extractText extracts clean text from the current page or a CSS selector.
// It removes script, style, noscript, and svg elements, then collapses whitespace.
func (b *Browser) extractText(ctx context.Context, selector string) (string, error) {
	jsCode := `
	(function(sel) {
		var el = sel ? document.querySelector(sel) : document.body;
		if (!el) return "";
		var clone = el.cloneNode(true);
		var tags = ["script", "style", "noscript", "svg"];
		for (var i = 0; i < tags.length; i++) {
			var elems = clone.querySelectorAll(tags[i]);
			for (var j = 0; j < elems.length; j++) {
				elems[j].remove();
			}
		}
		return clone.innerText || "";
	})(%s)
	`

	selArg := "null"
	if selector != "" {
		selArg = fmt.Sprintf("%q", selector)
	}

	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(jsCode, selArg), &text)); err != nil {
		return "", fmt.Errorf("browser: extract text: %w", err)
	}

	text = collapseWhitespace(text)

	if len(text) > maxContentBytes {
		text = text[:maxContentBytes] + "\n[content truncated]"
	}

	return text, nil
}

// pageInfo returns the current page URL and title.
func (b *Browser) pageInfo(ctx context.Context) (currentURL, title string, err error) {
	if err := chromedp.Run(ctx,
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
	); err != nil {
		return "", "", fmt.Errorf("browser: page info: %w", err)
	}
	return currentURL, title, nil
}

// multiBlankLine matches two or more consecutive newlines (with optional whitespace).
var multiBlankLine = regexp.MustCompile(`\n\s*\n`)

// collapseWhitespace reduces multiple blank lines to a single newline.
func collapseWhitespace(s string) string {
	return strings.TrimSpace(multiBlankLine.ReplaceAllString(s, "\n"))
}
