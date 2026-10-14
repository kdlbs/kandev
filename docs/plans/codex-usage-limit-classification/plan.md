---
status: planned
created: 2026-09-28
requirements:
  - REQ-AGENTS-CODEX-USAGE-LIMIT-001
system_design:
  - ../../specs/agents/system-design/codex-usage-limit-classification.md
---

# Codex usage-limit classification

## Overview

Codex ACP reports an exhausted account usage limit as a plain agent message
with a typographic apostrophe and returns only `Internal error` from
`session/prompt`. The quota rule can classify the notice, but the adapter did
not carry it onto the generic prompt error. Dynamic routing therefore could
not correlate its diagnostic with the terminal failure, and manual recovery
could discard a queued prompt. Preserve the notice on its matching generic
error, use the existing quota routing policy, and retain queued work.

## Technical approach

Accept both apostrophe forms in the codex quota rule so the plain notice
classifies as `quota_limited` with high confidence and allows fallback. The ACP
adapter records the sanitized notice on the active prompt, then attaches it to
only the matching generic `-32603` / `Internal error` after the notification
queue drains. Keep the original ACP error unwrap-able. Pass the provider ID
through orchestrator diagnostic and terminal classification so the existing
code-and-text correlation can authorize fallback. The wrapped message also
lets manual recovery retain queued work.

Apply text reset parsing only when no structured `ResetHint` exists and the
notice includes an explicit UTC zone or numeric offset. Reject malformed date,
time, and offset values. Resolve a yearless date to the next future occurrence
in its stated zone. The Codex notice currently has no zone, so it uses the
existing retry policy without a text-derived deadline.

A quota failure still opens the shared credential binding circuit, so sibling
profiles on the same account are skipped; a focused dynamic-engineering
regression proves that reuse is unchanged.

## Delivery order

1. Add failing tests for the explicit-zone parser, invalid date and clock
   values, and year rollover.
2. Add a failing ACP integration test for the notice followed by a generic
   prompt error. Project only that same-turn failure as a typed Codex error.
3. Add a failing orchestrator correlation test. Pass the provider ID into both
   diagnostic and terminal classification.
4. Run the focused routing, ACP, and orchestrator tests, then validate the
   updated specification files.

## Verification strategy

- `go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic
  ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator
  -count=1` covers classification, reset parsing, same-prompt ACP error
  projection, routing evidence, and manual recovery.
- `gofmt` formats the changed Go files.
- `python3 scripts/lint-spec-files.py --all` and
  `python3 scripts/list-docs.py validate` cover the specification package.
- `git diff --check` covers whitespace.

## Work orders

- [ ] [Task 01: Classify codex usage-limit notices](task-01-classify-codex-usage-limit.md)

## Risks and exclusions

- Unzoned provider wall-clock values must not become absolute circuit
  deadlines.
- Parsing must fail closed on invalid dates, clocks, and offsets.
- A structured reset hint must never be overwritten by text parsing.
- No provider rule other than codex-acp changes, and candidate ordering and
  the shared credential binding circuit are untouched.
