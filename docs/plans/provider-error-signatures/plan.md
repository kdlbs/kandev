---
created: 2026-10-08
status: implemented
updated: 2026-11-08
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

The prefixed OpenCode free-usage form has a separate
([task 03](task-03-prefixed-free-usage-followup.md)) work order. It adds
regression coverage for established error labels and records the signature's
provider scope, prose boundary, precedence, and reset-hint behavior.

## Scope

### In scope

- The provider rule patterns in `apps/backend/internal/agent/runtime/routingerr/rules.go`.
- Narrow coverage of the remaining signatures, using existing tests and
  disposable overlays outside the repository's automatic test collection.
- The `provider-neutral` labeled JSON signature in
  `apps/backend/internal/agent/runtime/routingerr/provider_neutral_rules.go`.
- Passing raw diagnostic text to the provider matcher only for explicitly
  marked signatures; the stored `RawExcerpt` remains sanitized.

### Out of scope

- Changes to prior rule order, confidence, or classification flags; new rules
  retain established higher-priority matches.
- Unrelated provider-neutral and all runtime-environment rules.
- How classified failures are attributed or acted on by dynamic routing.

## Verification

- `go test ./internal/agent/runtime/routingerr/`
- Package tests of the classifier consumers (`dynamic`, `registry`,
  `lifecycle`, `agentctl/server/adapter/transport/acp`, `office/scheduler`,
  `office/service`, `utility/handlers`, `orchestrator`) compared with the same
  packages on the base revision.
- `golangci-lint run ./internal/agent/runtime/routingerr/...`
- `make -C apps/backend build`

## Bounded catalogue follow-up

Add only the remaining OpenCode signatures: unlabelled `free usage limit
reached`, raw-text daily reasons `free-models-per-day` and `limit_rpd/<model>`,
and the generic Zen retry-later rate-limit wording. Add the shared labeled JSON
`error_type=provider_unavailable` opening signature. Keep provider-specific
scope, line-opening evidence, sanitized excerpts, typed/normalized precedence,
and existing credit/payment/quota ordering. Do not duplicate the already
submitted shared catalogue or context-length work. The original-source reproduction and focused candidate race result are recorded
in task 02; hosted CI and combined unmerged-source validation remain separate.

### Prefixed free-usage follow-up

Cover the established OpenCode error labels that can prefix `free usage limit
reached`. Keep the signature limited to OpenCode and to the start of a
diagnostic line, preserve structured and earlier account-limit precedence, and
do not infer a reset time. The production rule was already present in the PR;
task 03 adds committed regression tests for its accepted labels, rejected
prose and provider cases, and precedence.
