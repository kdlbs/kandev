---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
legacy_specs: []
---

# Implementation Plan: Classify provider limits by signature

## Overview

Several provider rules matched topic words anywhere in the text:
`claude.stderr.rate.v1` and the OpenCode, Copilot, and Amp rate rules used
`(?i)rate.?limit`, the OpenCode and Amp quota rules used `(?i)quota`, and the
Claude subscription rule used `(?i)subscription`. Agent prose that quoted
another session's error, for example a coordinator reporting that a child hit
"Rate limit exceeded", was classified as the reporting agent's own provider
limit and suspended a healthy account.

One work order replaces the topic-word patterns with provider error
signatures. See [task 01](task-01-match-provider-signatures.md).

## Scope

### In scope

- The provider rule patterns in `apps/backend/internal/agent/runtime/routingerr/rules.go`.
- Table tests for prose that must not classify and signatures that must.
- The narrow context-length follow-up in `provider_neutral_rules.go`, its
  phase-aware call in `routingerr.go`, and the existing exact consumer in
  `orchestrator/event_handlers_transient_messages.go`.

### Out of scope

- Rule order, confidence, and classification flags.
- Other provider-neutral and runtime-environment rules.
- How classified failures are attributed or acted on by dynamic routing.

## Verification

- `go test ./internal/agent/runtime/routingerr/`
- Package tests of the classifier consumers (`dynamic`, `registry`,
  `lifecycle`, `agentctl/server/adapter/transport/acp`, `office/scheduler`,
  `office/service`, `utility/handlers`, `orchestrator`) compared with the same
  packages on the base revision.
- `golangci-lint run ./internal/agent/runtime/routingerr/...`
- `make -C apps/backend build`

## Context-length producer/consumer follow-up

The original work above remains implemented. Criterion 001.7 adds only a named
prompt-send context-length producer and its existing exact pre-result consumer.
A producer without the consumer fix would leave the formerly anonymous failure
outside the accepted route; a consumer without the producer absent from main
would be unused compatibility code. Both pieces are prepared together.

Startup classification, typed-limit priority, false global retry/fallback
flags and diagnostic/output/effect fences stay intact. The separate structured-
envelope criterion 001.6 and remaining catalogue/quota/notice work are not ported.
Disposable baseline and producer-only entrypoint checks failed as expected:
the former had no named producer (0.023s), and the latter returned the named
runtime error outside the accepted exact consumer (0.062s). The complete
candidate and selected existing controls passed once under `-race`:
`routingerr` 1.285s, `dynamic` 1.048s and `orchestrator` 3.003s. This includes
startup/later-phase preservation, typed-limit priority, diagnostic/output/effect
fences, repeat-result stability, ordinary manual recovery and workflow-claim
fences. Temporary overlays are removed after delivery. No standing test,
whole-build requirement or operating mutation is added.
