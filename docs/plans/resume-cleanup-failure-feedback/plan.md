---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Resume cleanup failure feedback

## Overview

Expose the cleanup failure that prevents session Resume, using the existing
recovery error surface. Deliver safe backend failure evidence first, then
localized desktop and phone presentation. The user approved this design package and then explicitly authorized
implementation and PR delivery.

Agents owns Resume causality and presentation. Tasks remains the durable error
storage owner, and Executors retains Kubernetes resource ownership. Existing
requirements 002.1/.8/.9 and the presentation contract already require truthful
causes. The 002.10-.13 amendment makes cleanup behavior explicit.

## Confirmed root cause and reproduction

A diagnostic bundle collected October 10 had
no manifest warnings. For the reported Kubernetes-backed task, the agent
stream disconnected October 9 at 15:28:36 Lisbon time. Cleanup on October 9
failed with Kubernetes API connection refusal. October 10 attempts reached
the API but failed port forwarding to the pod's node kubelet with a 502.
The same execution remained registered throughout the failed attempts.

Reproduced before this change in running build
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8` and the checkout: terminal preflight
and already-registered fallback logged cleanup errors and continued. `cleanupStaleExecution` correctly retains the registration when
stop cannot be confirmed. A subsequent launch therefore replaces the useful
cleanup cause with `ErrAgentAlreadyRunning`. This is not proof of another
live agent or a second session.

Faithful regression: use the existing Resume fixture with a registered stale
execution; make cleanup return a wrapped connection failure. In terminal state,
assert no launch occurs. In a nonterminal fallback, make initial launch return
the registration conflict, report no live agent, and fail cleanup; assert there
is no second launch. Repeat with successful cleanup and a genuinely live agent.

## Scope

### In scope

- Stop at either failed cleanup boundary; retain error-chain and ownership evidence.
- Safe typed request and durable cause, with attempt-specific history and reload.
- Existing recovery card/error feedback, localization, safe Details/Copy, retry state.
- Focused backend, frontend, desktop and mobile regressions; public recovery guidance.

### Out of scope

Cluster repair, node eviction, Pod/PVC deletion or migration, forced record removal,
automatic fresh conversations, new recovery actions, resource scope redesign,
new feature flags, broad recovery UI redesign, and live-instance mutations.

## Technical approach

### Task 01: Preserve cleanup evidence

Change `Executor.resumeSession` at both cleanup boundaries. Terminal preflight
returns before request assembly or `STARTING`; fallback failure uses the owned
rollback path without retrying launch. Keep live-concurrent success and successful
cleanup behavior. Preserve cancellation semantics and credentials/native token.

Use `runtime.BootstrapFailure` and the existing `AgentErrorCause` shape with
additive code `runtime_cleanup_failed`, operation `resume`, fixed safe detail,
and proof of no dispatch for that attempt. Extend allowlists and normalization.
Raw errors remain backend evidence. Do not make HTTP/WS clients receive the raw
wrapped error just because `errors.As` preserves it.

Integrate durable `LastAgentError`, existing resume/error stamps and history;
guard early terminal errors by current observed state, stamp, and owned host
resume attempt rather than fabricating a startup attempt. Existing session
launch/recovery response mappings carry safe cause and identity. The type must
survive automatic resume and an independent workspace-restore failure.

### Task 02: Render truthful guidance

Extend browser cause parsing/types and request-local recovery evidence.
`session-bootstrap-recovery-model.ts` chooses localized copy for the new cause;
`SessionRecoveryCard` keeps its composer-region placement and existing actions.
Historical rows remain readable without duplicate controls. Details and clipboard
share safe content. Do not infer Kubernetes, network, or node-death causes by
matching arbitrary error strings. Preserve legacy generic fallback.

Update `docs/public/sessions-and-review.md` and the recovery section of
`docs/public/k8s.md` with the implemented meaning and connectivity prerequisite.
These are how-to recovery instructions. Search the root README and screenshot
catalog for contradictions; no screenshot replacement is expected for a copy-only
change in the existing surface. Public recovery instructions now explain the connectivity prerequisite.

### Compatibility matrix

| Path | Failure evidence | Result | Proof |
| --- | --- | --- | --- |
| Kubernetes terminal Resume | Failed old-instance cleanup | No replacement; safe cleanup cause | Executor regression and projection/handler test |
| Nonterminal stale registration | Initial conflict, no live agent, failed cleanup | No second launch; owned rollback | Executor fallback regression |
| Other executors | Typed cleanup failure at the same boundary | Same safe cleanup explanation; no invented network claim | Generic fake-runtime table |
| Live concurrent execution | Existing live guard | Existing success/admission behavior | Existing concurrent-resume regression |
| Successful cleanup | Confirmed cleanup | Normal same-session Resume | Positive-control regression |
| Legacy or unknown browser cause | No typed cleanup evidence | Existing fallback | Browser model regression |

## ASCII UI preview

Illustrative wording, localized during implementation. Required structure is
title, short cause/prerequisite, existing eligible actions, then Details.
This is the session error surface; unrelated shared task errors keep their strip.

### UI-01: Current error text

```text
Task failed to start
session already has an agent running: session "..." (execution: ...)
```

The reported toast text is confirmed by logs. It does not prove the exact
current error-card geometry.

### UI-02: Desktop cleanup failure

```text
[!] Resume could not finish
The previous runtime could not be stopped.
Check executor connectivity, then retry Resume.
[Resume] [other existing eligible actions]
> Details
```

### UI-03: Phone cleanup failure

```text
[!] Resume could not finish
The previous runtime could not
be stopped. Check executor
connectivity, then retry Resume.
[          Resume           ]
[ existing eligible action  ]
> Details
```

Reuse the shipped `SessionRecoveryCard` and `task-layout.tsx` composition;
phones stack actions with 44px touch targets. Desktop actions wrap. Expanded
details have safe operation/cause/time fields and Copy, under the same
recovery-region scroll owner. Preserve viewport/safe-area handling. Retry shows
pending status and disables equivalent actions; failed retry retains guidance.
Successful owned retry restores the composer and its draft. No new drawer is
needed for this short session cause. Map to 002.5/.8/.9/.12/.13.

## Tests

Task 01 adds `TestResumeSessionCleanupFailure` table cases for both boundaries,
`TestSessionCleanupFailureProjection`, and `TestSessionCleanupFailureResponse`.
Cover 002.10/.11/.12/.13 plus token/rollback behavior from 001.1/.5.
Reuse lifecycle retained-stop tests rather than changing cleanup policy.

Task 02 adds typed-cause, legacy-fallback, duplicate-owner and safe-copy tests
beside the recovery model/card tests. Verify both resume and restore causes when
different, and stale failure/success callbacks. Map to 002.8/.9/.12/.13.

## E2E tests

Add a shared mock-backed cleanup failure scenario to
`e2e/helpers/session-error-recovery-ui.ts`, used by existing desktop
`session-error-recovery-ui.spec.ts` and `mobile-session-error-recovery-ui.spec.ts`.
Exercise repeated failed Resume, pending, Details/Copy, reload, and a subsequent
successful owned retry. Assert one active surface, preserved draft/history,
unchanged session identity, wrapped text, no horizontal overflow, and mobile
44px targets. Inspect a rendered phone screenshot against UI-03. No real cluster
is required; this tests feedback and not Kubernetes transport implementation.

## Work orders

- [x] [Task 01: Preserve and project cleanup failures](task-01-preserve-cleanup-failures.md)
- [x] [Task 02: Explain cleanup failure in recovery UI](task-02-render-cleanup-feedback.md)

Execute sequentially. No subagents are authorized.

## Verification results

Implementation checks passed on October 10:

- Both executor cleanup paths, SQLite durable projection and actual launch/recover
  handlers passed regression checks. Projection ownership cases also passed under
  the Go race detector. Pinned Go lint reported zero issues.
- Frontend model/service/manual/automatic/history/owner regressions, typecheck,
  changed-file ESLint, complete localization and new-copy ratchet passed.
- Final desktop and phone recovery suites: ten passed per viewport, zero retries.
  Inspected synthetic screenshots confirm wrapping, native phone actions and
  collapsed details. No live incident runtime was mutated.
- Existing automatic Files navigation failed on the untouched base. A focused
  owner-correlation fix now keeps the matching session recovery link when
  workspace readiness supersedes restoration, preserving independent errors.
- Public documentation validation and tests passed. Catalog, full spec lint,
  diff checks and actual-path deterministic PR documentation coverage passed.

Design-package checks passed at the earlier checkpoint:

- `python3 scripts/list-docs.py validate`: 369 decisions and 1517 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/resume-cleanup-failure-feedback`: passed.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage` with the planned
  backend/frontend production paths and actual package contents: `covered` for
  both work orders. This is prospective reference coverage, not a live PR check.
- Referenced existing frontend verification inputs and public pages exist.

The requirement remains below its 20 KiB limit after removing duplicate migrated
incident prose and two examples already covered by 001.8 and 002.1. Existing
acceptance criteria remain unchanged. These were the design-checkpoint results before implementation. The live
incident runtime was not started or mutated.

## Risks

- Returning early must not leave an owned STARTING attempt or overwrite a newer state.
- Existing `BootstrapFailure.Error()` exposes the wrapped cause; browser projections
  must deliberately select safe fields at every response/persistence boundary.
- Automatic and manual feedback can race with durable error events. Correlate by
  attempt and stamp, not matching text or session alone.
- Connectivity guidance does not diagnose the node or authorize removal of a
  retained workload; ownership refusal must remain an honest cleanup failure.
- Existing old errors cannot recover a discarded cleanup cause without evidence.
