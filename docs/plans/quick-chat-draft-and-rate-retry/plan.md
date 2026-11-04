---
created: 2026-10-26
status: done
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
  - REQ-AGENTS-CODEX-RATE-SIGNATURE-001
  - REQ-PLATFORM-RATE-THROTTLE-RETRY-001
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
  - ../../specs/agents/system-design/provider-error-signatures.md
  - ../../specs/platform/system-design/provider-error-recovery.md
legacy_specs: []
---

# Implementation plan: Quick Chat draft clearing and rate-limit retry

## Overview

Restore exact opening-prompt cleanup, then classify terminal Codex throttles and
admit eligible interactive retries. Execute the two work orders sequentially.
The direct task request authorizes implementation of this package; the
work-order statuses below track execution.

## Evidence and requirement conformance

A temporary real TipTap test restored `first paragraph\n\nsecond paragraph`
through `textToHtml`. Serialization produced three intervening newlines.
`clearAcceptedPayload` compares trimmed text, so this mismatch prevents clearing.
This violates existing AC-TASKS-QUICK-CHAT-COMPOSER-001.10; preserve .9 recovery.
The repair changes conversion, not the snapshot ownership check.

A temporary Go test classified the exact screenshot error,
`exceeded retry limit, last status: 429 Too Many Requests`, as
`agent_runtime_error` with manual recovery. Structured HTTP 429 classified as
`rate_limited`, also manual without a reset hint. The signature is missing;
missing-hint retry eligibility is an explicit policy extension.

The running bundle reported commit `b6263ea4c3165c86ec60e29e798475ad552ad6ce`.
Relevant composer files matched this checkout. Ordinary backend logs did not
contain the exact failure, so no captured ACP envelope is claimed. Temporary
reproduction tests were removed after passing. Product regression tests remain
pending. No live instance was mutated.

## Scope

### In scope

- Lossless restoration and matching accepted-payload cleanup.
- Anchored Codex error signature and negative collision coverage.
- High-confidence missing-hint interactive throttle eligibility.
- Existing replay fences, retry budget, cancellation, and visible recovery.
- Desktop and phone regression scenarios.

### Out of scope

Office policy changes, dynamic policy changes, provider switching, raw ACP Data
classification, new continuation capabilities, layout redesign, and new settings.

## Technical approach

Task 01 changes shared text restoration in `use-tiptap-editor.ts` using
`textToEditorContent`, with actual editor and acceptance integration coverage.
Preserve rich-document precedence, attachments, session identity, and newer edits.
Audit other callers of `textToHtml`; change only paths implicated by the regression.

Task 02 changes `routingerr/rules.go` and context-aware `routingerr/policy.go`.
Use existing orchestration admission and scheduling. Add no provider branch in
orchestration and no browser retry timer. Verify the exact terminal message
through the normal adapter projection and orchestrator path, not only the regex.

| Path | Intended result | Evidence / unsupported fallback |
| --- | --- | --- |
| Codex ACP terminal message, exact signature | Rate classification; eligible safe interactive retry | Catalogue, terminal-path integration, mock E2E |
| Other high-confidence rate classification | Same interactive missing-hint rule | Context policy matrix |
| Quota/auth/unknown or Data-only signature | Existing manual/hard policy | Collision and projection regressions |
| Provider internally retrying, unsettled turn | No second retry owner | Terminal settlement integration test |
| Office / dynamic / utility / passthrough | Existing consumer policy | Context tests; no new support implied |
| Supported post-output continuation | Existing capability gates | Mixed completed/unresolved tool refusal test |

## ASCII UI preview

UI-01: Accepted Quick Chat opening message. Shared content order on desktop
and phone; desktop uses the existing dialog, phone the existing full-height chat.

```text
Before: transcript [opening message]    composer [opening message]
After:  transcript [opening message]    composer [empty, or newer draft]
Retry:  transcript [opening message]
        [Rate limit | attempt 1/5 | countdown | Cancel]
        composer [empty, or newer draft]
```

The transcript scrolls; the existing composer remains at the bottom. Keep phone
safe-area handling and touch targets. Reuse existing localized recovery copy.
The empty accepted draft and single retry notice are required; wording above is
illustrative. No new layout, navigation, picker, or control is proposed.

## Tests

Task 01 maps composer AC .9/.10 to helper round-trip, mounted editor cleanup,
newer-draft, attachment, rich-content, and session-switch regressions.
Task 02 maps signature AC .1-.3 to exact/prefixed and negative fixtures.
It maps throttle AC .1-.5 to policy, terminal-path, budget, and ownership tests.
Every owned command and exact test location is listed in its work order.

## E2E tests

Extend `quick-chat-opening-composer.spec.ts` and its `mobile-` counterpart for
blank-line submission, one accepted message, empty composer, and reload.
Extend `transient-retry.spec.ts` and its `mobile-` counterpart for terminal Codex
429, one countdown notice, cancellation, and successful retry without duplication.
Use mock terminal errors through the real backend. Never call live providers.
Desktop runs in `chromium`; phone runs in `mobile-chrome`.

## Work orders

- [x] [Task 01: Preserve opening prompt text](task-01-opening-draft.md)
- [x] [Task 02: Retry terminal rate throttles](task-02-rate-throttle.md)

Task 02 follows Task 01 for the combined Quick Chat acceptance scenario.
Execution is sequential and does not authorize delegation.

## Existing delivery records

The original `quick-chat-opening-composer` package is implemented;
`provider-error-policies` is done. Their historical results remain historical.
This package records the newly found regression and policy extension. It does
not reopen prior work orders or claim their tests cover these new scenarios.

## Verification results

Design validation on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed (403 decisions, 1741 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage`: covered, no errors.
  The check loaded both work orders and their referenced documents, with a
  representative planned source path to exercise the delivery coverage gate.
- Work orders and their specification links are present; files remain unstaged.

Implementation verification on 2026-10-26:

- Backend package tests, changed-Go lint, and backend build passed.
- The complete desktop transient-retry suite passed 6 tests; the phone suite
  passed 3 tests. Focused Quick Chat retry checks also passed after extracting
  the shared message surface.
- The draft-ownership and multiline-restoration Vitest files passed 29 tests.
- Web typecheck, production build, changed-file ESLint, and Prettier passed.
- The public developer-tools guide now explains safe bounded Quick Chat
  rate-limit retry and the visible progress and cancel controls.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, public-doc tests and validation,
  and `git diff --check` passed.

## Risks

- Broad whitespace normalization could erase a legitimate newer draft.
- Shared editor conversion could alter mentions, code, or saved-prompt tokens.
- A context-free retry change could alter Office scheduling.
- Rate classification alone cannot authorize replay after output or effects.
- Synthetic terminal fixtures prove the supported path, not unobserved native
  Codex protocol variants. Preserve manual recovery for unsupported evidence.
