# Silent Empty Message on Succeeded Batch Results

## Severity: Critical

## Location

- `pkg/providers/anthropic/batch.go:186` — `convertResult`

## Description

`convertResult` calls `b.adapter.parseResponse(item.Result.Message)` for any non-`"errored"` result without checking whether the content block slice is empty. An Anthropic batch response with `"type": "succeeded"` but no content blocks returns a zero-`Parts` message with no error.

The OpenAI-compatible batch path (`internal/openaicompat/batch.go:275`) explicitly guards `len(resp.Choices) == 0` with an error; this path does not.

The agent receives a silent empty message and proceeds as if the LLM replied with nothing.

## Fix

Add a guard after the error-type check:

```go
if len(item.Result.Message.Content) == 0 {
    return batch.Result{
        Err: fmt.Errorf("anthropic batch: request %s: empty content in response", item.CustomID),
    }
}
```
