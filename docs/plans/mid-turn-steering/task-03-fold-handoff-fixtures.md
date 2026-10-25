---
id: "03-fold-handoff-fixtures"
title: "Capture fold-handoff wire fixtures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 03: Capture fold-handoff wire fixtures

- **Acceptance:** The observed fold-handoff sequence exists as a committed
  fixture and an adapter test replays it: predecessor `session/prompt` resolves
  `end_turn` with zeroed usage and no answer text, the answer and the real usage
  arrive on the successor prompt, and only one `sleep` tool call occurs.
- **Acceptance:** A second fixture covers the background variant — a
  `run_in_background` launch card reporting `completed` long before the workload
  ends, and the workload's completion arriving as assistant text with **no**
  `tool_call_update` while no prompt is in flight.
- **Verification:** `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/...`
- **Files likely touched:**
  `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/`
  (new fixture files alongside `acp-messages.jsonl`), a focused replay test in
  the same package.
- **Dependencies:** None. This task is evidence capture only and must not change
  adapter behavior.
- **Inputs:** `scripts/probe-acp-midturn-fold` produces both transcripts; its
  header documents the background invocation. Recorded runs are available and
  reproducible. ADR-0049 admits handoff only for bridges that are
  *fixture-tested* — the probes are evidence, these fixtures are the contract.
  Existing `testdata/acp-messages.jsonl` uses a `{key, input, expected}` shape
  for normalization cases; a raw frame transcript is a different shape, so add a
  new fixture form rather than forcing it into that one.
- **Risks:** Fixtures pin an undocumented, `@internal` upstream behavior that can
  change without a changelog entry. Assert the *shape* the adapter must survive
  (early settle, zeroed usage, later text) rather than exact timings, token
  counts, tool ids, or session ids.
- **Output contract:** Report the fixture paths and shape, which spec failure
  modes each one pins, exact commands/results, and update only this task's
  status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/...`: passed.
- Fixtures live under
  `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/` and are
  replayed by `fold_handoff_fixture_test.go`, pinning the observed fold-handoff
  wire shape and background-work survival across the turn boundary.
