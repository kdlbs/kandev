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

The original [task 01](task-01-match-provider-signatures.md) replaced the
topic-word patterns with provider error signatures. A bounded follow-up now
([task 02](task-02-provider-catalogue-followup.md)) covers remaining provider
text forms and one labeled JSON error envelope; it
does not reopen task 01's completed prose-safety change.

## Scope

### In scope

- The provider rule patterns in `apps/backend/internal/agent/runtime/routingerr/rules.go`.
- Table tests for prose that must not classify and signatures that must.
- Narrow coverage of the remaining signatures, using existing tests and
  disposable overlays outside the repository's automatic test collection.
- The shared provider catalogue in
  `apps/backend/internal/agent/runtime/routingerr/provider_catalog_rules.go` and
  the `provider-neutral` labeled JSON signature in
  `apps/backend/internal/agent/runtime/routingerr/provider_neutral_rules.go`.
- The narrow context-length follow-up in `provider_neutral_rules.go`, its
  phase-aware call in `routingerr.go`, and the existing exact consumer in
  `orchestrator/event_handlers_transient_messages.go`.
- Passing raw diagnostic text to the provider matcher only for explicitly
  marked signatures; the stored `RawExcerpt` remains sanitized.

### Out of scope

- Changes to prior rule order, confidence, or classification flags; existing
  rules keep their order, confidence, and flags, and new rules retain
  established higher-priority matches.
- Unrelated or other provider-neutral rules and all runtime-environment rules;
  the named prompt-send context-length signature (criterion 001.7) is in scope.
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
envelope criterion 001.6 and remaining catalogue/quota/notice work arrive from
current upstream main's bounded catalogue follow-up (below) and stay a distinct
rule set rather than being folded into this shared catalogue. Both rule sets
coexist in the neutral table, and the context-length rule stays prompt-send only.
Disposable baseline and producer-only entrypoint checks failed as expected:
the former had no named producer (0.023s), and the latter returned the named
runtime error outside the accepted exact consumer (0.062s). The complete
candidate and selected existing controls passed once under `-race`:
`routingerr` 1.285s, `dynamic` 1.048s and `orchestrator` 3.003s. This includes
startup/later-phase preservation, typed-limit priority, diagnostic/output/effect
fences, repeat-result stability, ordinary manual recovery and workflow-claim
fences. Temporary overlays are removed after delivery. No standing test,
whole-build requirement or operating mutation is added.

## Bounded catalogue follow-up

Add only the remaining OpenCode signatures: unlabelled `free usage limit
reached`, raw-text daily reasons `free-models-per-day` and `limit_rpd/<model>`,
and the generic Zen retry-later rate-limit wording. Add the shared labeled JSON
`error_type=provider_unavailable` opening signature. Keep provider-specific
scope, line-opening evidence, sanitized excerpts, typed/normalized precedence,
and existing credit/payment/quota ordering. Do not duplicate the already
submitted shared catalogue or context-length work. The original-source reproduction and focused candidate race result are recorded
in task 02; hosted CI and combined-source validation after the merge are separate.

## Catalog consolidation follow-up

Status: in progress. Task 01 now carries the literal provider catalog and
bounded request-rejection consumer follow-up. Existing completed work and
verification remain valid for their original scope. The signature disposition
and changed-source boundaries are recorded in Task 01. The prepared
thinking-only empty-completion change is integrated with the upstream
diagnostic signature preserved. Review and publisher acceptance remain pending;
local implementation is not evidence of publication or account-limit recovery.
After the current-main synchronization both rule sets are present: this branch's
shared catalogue (task 01) and upstream main's bounded catalogue (task 02).
