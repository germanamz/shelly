# Handoff Peer Receives Wrong Context in `autoAnswer`

## Severity: Important

## Location

- `pkg/agent/delegation.go:519-542`

## Description

When `handleHandoff` spawns a peer agent, it wires `peer.interaction = NewInteractionChannel()` and calls `runChildWithHandoff(ctx, a, peer, t, handoffCount+1)`. Inside `runChildWithHandoff`, `autoAnswer` is started with `t.Context`, which is the original parent delegation context — not the handoff context (`hr.Context`).

The peer's chat already contains the `<handoff_context>` block from `hr.Context`, but if the peer calls `request_input`, it receives answers synthesized from the original task context. For any non-trivial handoff where the work context has changed, this produces misleading answers.

## Fix

Pass `hr.Context` as `t.Context` when calling `runChildWithHandoff` for handoff peers, or create a modified `delegateTask` with `Context: hr.Context`.
