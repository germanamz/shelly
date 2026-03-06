# `cache_control` Placed on Request Envelope Instead of Content Blocks

## Severity: Important

## Location

- `pkg/providers/anthropic/anthropic.go:93` — `apiRequest` struct definition
- `pkg/providers/anthropic/anthropic.go:154` — field always set to `{type:"ephemeral"}`

## Description

The `apiRequest` struct has a top-level `CacheControl` field that is always set to `{type:"ephemeral"}`. Per the Anthropic API documentation, `cache_control` is a property of individual content blocks (tools, system messages, individual message blocks), not the request root.

If the top-level field is silently ignored by the API, prompt caching is entirely inactive despite the code's intent. `CacheCreationInputTokens` and `CacheReadInputTokens` in the usage tracker will always be zero. The provider README explicitly documents this feature as intentional behavior.

## Fix

Remove the top-level `CacheControl` field from `apiRequest`. Instead, attach `cache_control: {type: "ephemeral"}` to the appropriate content blocks:
- The last tool definition in the tools array
- The system message block
- Or the last user message block

Per Anthropic docs, the API caches the entire prefix up to the last block marked with `cache_control`.
