package codingtoolbox

import "context"

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)
