package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/germanamz/shelly/pkg/chats/message"
)

// Runner executes agent logic and returns the final message.
type Runner interface {
	Run(ctx context.Context) (message.Message, error)
}

// RunnerFunc adapts a plain function to the Runner interface.
type RunnerFunc func(ctx context.Context) (message.Message, error)

// Run calls the underlying function.
func (f RunnerFunc) Run(ctx context.Context) (message.Message, error) {
	return f(ctx)
}

// Middleware wraps a Runner, returning a new Runner with added behaviour.
type Middleware func(next Runner) Runner

// --- Timeout middleware ---

// Timeout returns a Middleware that wraps the runner's context with a deadline.
func Timeout(d time.Duration) Middleware {
	return func(next Runner) Runner {
		return RunnerFunc(func(ctx context.Context) (message.Message, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			return next.Run(ctx)
		})
	}
}

// --- Recovery middleware ---

// Recovery returns a Middleware that catches panics and converts them to errors.
func Recovery() Middleware {
	return func(next Runner) Runner {
		return RunnerFunc(func(ctx context.Context) (msg message.Message, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("agent panicked: %v", r)
				}
			}()

			return next.Run(ctx)
		})
	}
}

// --- Logger middleware ---

// Logger returns a Middleware that logs agent start, duration, and error.
func Logger(log *slog.Logger, name string) Middleware {
	return func(next Runner) Runner {
		return RunnerFunc(func(ctx context.Context) (message.Message, error) {
			log.InfoContext(ctx, "agent started", "agent", name)

			start := time.Now()

			msg, err := next.Run(ctx)

			duration := time.Since(start)

			if err != nil {
				log.ErrorContext(ctx, "agent finished with error",
					"agent", name,
					"duration", duration,
					"error", err,
				)
			} else {
				log.InfoContext(ctx, "agent finished",
					"agent", name,
					"duration", duration,
				)
			}

			return msg, err
		})
	}
}

// --- OutputGuardrail middleware ---

// OutputGuardrail returns a Middleware that validates the final message. If
// check returns an error, that error is returned instead of the message.
func OutputGuardrail(check func(message.Message) error) Middleware {
	return func(next Runner) Runner {
		return RunnerFunc(func(ctx context.Context) (message.Message, error) {
			msg, err := next.Run(ctx)
			if err != nil {
				return msg, err
			}

			if checkErr := check(msg); checkErr != nil {
				return message.Message{}, checkErr
			}

			return msg, nil
		})
	}
}
