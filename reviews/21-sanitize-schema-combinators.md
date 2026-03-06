# `sanitizeSchema` Doesn't Recurse Into `allOf`/`anyOf`/`oneOf`

## Severity: Minor

## Location

- `pkg/providers/gemini/gemini.go:309-341`

## Description

`sanitizeSchema` strips `$schema` and `additionalProperties` recursively from `properties` and `items` but not from `allOf`, `anyOf`, or `oneOf` arrays.

Any tool that registers a manually constructed schema using these combinators will send unsanitized nested schemas to the Gemini API, which rejects `additionalProperties`.

The current `pkg/tools/schema` generator does not emit these combinators, so this is not triggered by built-in tools today but is a latent defect for externally registered tools.

## Fix

Add recursion into `allOf`, `anyOf`, and `oneOf` arrays in `sanitizeSchema`:

```go
for _, key := range []string{"allOf", "anyOf", "oneOf"} {
    if arr, ok := m[key].([]any); ok {
        for _, item := range arr {
            if sub, ok := item.(map[string]any); ok {
                sanitizeSchema(sub)
            }
        }
    }
}
```
