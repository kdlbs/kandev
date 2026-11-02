---
created: 2026-10-26
status: complete
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation Plan: Cursor continuation with pending foreground tools

## Overview

Resolve [issue #4801](https://github.com/kdlbs/kandev/issues/4801) under the
user's explicit agent-managed recovery policy. Platform owns shared recovery
admission and feedback; Agents supplies versioned prompt evidence.
First implement runtime admission and dispatch. Then implement truthful feedback
and desktop/mobile integration coverage. Execute sequentially in the main session.

## Evidence and requirement change

Current `ContinuationSafetySnapshot.SafeFor` rejects `Pending`. The producer
combines unfinished tools and unresolved permissions in that field. Admission
therefore refuses the mixed-tool turn before any retry starts. Retained failure
feedback maps `refused` to generic stopped copy. Existing tests encode this
intentional old policy; the change amends the contract rather than fixes an
accidental fingerprint mismatch.

[Native evidence](https://github.com/kdlbs/kandev/issues/4801#issuecomment-6418179442)
shows surviving tool execution and missing terminal results. Same-session
continuation can repeat work. The accepted
[decision](../../decisions/2026-10-26-cursor-pending-tool-continuation.md)
delegates that choice to Cursor. Do not treat duplicate prevention as an acceptance
gate or promise that native history contains every tool result.

## Scope

### In scope

- Versioned V3 evidence, separating permissions from pending foreground tools.
- Retained Cursor transport-loss continuation using exact hidden `continue`.
- Existing episode budget, ownership, Stop, and user-message priority.
- Accurate remaining refusal reasons and actual attempts on desktop and phone.
- Public guidance about agent-managed inspection and possible repeated effects.

### Out of scope

- Generic tool reconciliation, command heuristics, process joins or tool killing.
- Original-request replay after effects, other providers, capacity-policy changes,
  runtime replacement with pending tools, background/subagent recovery.
- New configuration, release toggle, migration, scheduler, or permission dialog.

## Technical approach

Use the existing streams snapshot, ACP ledger, orchestrator continuation binding,
retained failure projection, and feedback helper. See the design's
“Agent-managed pending-tool policy” section for the exact wire contract.
`SafeFor` must retain V1/V2 semantics. Permission resolution must explicitly attest
V3 eligibility; old helper omission fails closed. Bind pending recovery to its
retained execution and prohibit restoration at every later preparation boundary.

| Shape | Behavior | Evidence |
| --- | --- | --- |
| Cursor V3, retained, transport loss, valid pending foreground work | Continue | Mixed-tool unit and integration fixture |
| V3 without pending work | Existing completed-tool recovery | Completed-tool regressions |
| V1/V2 pending, omitted or unknown support | Refuse | Version-skew tests |
| Permission denial, missing permission resolution, invalid tools, background work | Refuse | Negative ledger and admission tests |
| Lost/replaced runtime during pending recovery | Refuse without restore | Timer/preparation race tests |
| Other error category/provider, dynamic/Office/utility/passthrough | Existing policy | Admission controls |

## ASCII UI preview

UI-01: Task Chat, retained Cursor transport interruption. Shared desktop/phone
content, reusing `TransientRetryNotice` and existing failure cards.

```text
Before: pending foreground tool
Automatic retry stopped. You can send another message.

After: eligible pending foreground tool
Cursor: Continuing in 5 seconds
Attempt 1 of 5                         [Cancel]

After: remaining refusal, zero attempts
Automatic continuation did not start.
Kandev cannot continue automatically because conversation or safety evidence is missing.
Review the work before sending another message.
```

The words are illustrative localization source copy. Phone keeps the existing
stacked Cancel target of at least 44px. Desktop retains compact inline controls.
Chat owns scrolling; diagnostics wrap without horizontal page overflow. No modal
or new confirmation is added. Existing task Chat is the mobile exemplar; its
continuation card supplies hierarchy, focus behavior, and status announcements.
Map UI-01 to `AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1` through `003.4`.

## Tests

Task 01 adds RED regressions named `TestCursorContinuationEvidencePendingV3`,
`TestInterruptionContinuationPendingToolsRetained`, and
`TestInterruptionContinuationPendingRuntimeLoss`. These map to `001.8`, `001.9`,
`001.3` through `001.5`, and `002.1` through `002.4`.
Cover completed + pending, pending-only, completed + pending + failed, late
completion, orphan updates, conflicting states, permissions before tool frames,
user Stop, stale generation, and background ownership. Verify host original=1,
continuation=1, exact `continue`, no user-message row, and same native/execution ID.
Do not assert an arbitrary model executes each tool once.

Task 02 extends `interruption-recovery-feedback.test.ts` and
`provider_interruption_refusal_test.go` for `003.2`. Cover absent historical
metadata, zero-attempt refusal, positive-attempt refusal, cancellation, and true
exhaustion. Existing capacity feedback remains independent.

## E2E tests

Extend `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`
and `mobile-provider-interruption-continuation.spec.ts`, with the shared helper
and mock producer. Use `chromium` and `mobile-chrome` respectively. Prove automatic
pending-tool continuation, no original replay, hidden internal prompt, Cancel
while waiting, new-user-message priority, accurate refusal, and reload behavior.
Retain malformed/unknown and legacy support refusal fixtures. Cover mobile
wrapped copy, reachable Cancel, and no horizontal overflow. These map to
`001.8`, `001.9`, `002.1` through `002.4`, and `003.1` through `003.4`.

## Work orders

- [x] [Task 01: Admit pending tools on retained Cursor runtimes](task-01-runtime-policy.md)
- [x] [Task 02: Verify feedback and desktop/mobile behavior](task-02-feedback-and-proof.md)

## Verification results

Implementation complete on 2026-10-26. Native report limitations remain explicit.

- Backend build: `make -C apps/backend build` passed.
- Backend regression tests passed for streams/ACP continuation evidence,
  orchestrator interruption/transient/retained continuation, lifecycle/watcher
  propagation, mock-agent fixtures, refusal metadata, and attempt counts.
- Changed-Go lint passed with `bash scripts/lint-go-changed` over all 26 changed
  Go paths recorded in Task 01 Results.
- Frontend: typecheck passed; ESLint passed on changed TS/TSX paths; Vitest passed
  74 tests across feedback, action-message, and E2E helper suites.
- Localization: `pnpm run i18n:zh-hant`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Browser: desktop continuation E2E passed 16/16; mobile continuation E2E passed
  4/4, including pre-dispatch cancellation and legacy refusal.
- Public docs: validator tests passed 63/63 and all 54 published pages
  validated.
- Specifications: `python3 scripts/list-docs.py validate` passed (404 decisions,
  1740 specifications); `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

Design validation on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed (404 decisions, 1740 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: covered,
  both work orders accepted. Input included all changed documents plus the
  planned streams runtime path to exercise reference coverage before code exists.

The public recovery guidance now separates transport-loss continuation from
capacity recovery and states the possibility of repeated tool effects.

Review follow-up on 2026-10-26 preserved scoped authorization through pending-
tool continuation retries and made retained refusal wording match the available
reason and attempt evidence. Task 01 and Task 02 record the focused checks and
results. The unfiltered orchestrator package run timed out in the unrelated
scheduling-orphan test; its isolated rerun and the plan's focused continuation
test set passed.

## Risks

- Cursor can repeat or overlap tool effects; this is an accepted policy limit.
- Conflating pending permissions and tools would bypass a separate permission guard.
- Remote helper skew must refuse rather than reinterpret legacy snapshots.
- A retained-runtime receipt can become stale before dispatch; revalidate it.
- Late ACP frames lack prompt generation; preserve existing ambiguity guards.
- Open PRs #4597 and #4712 touch adjacent recovery paths. Their scope and current
  state were checked before implementation; this package does not depend on
  either PR's unmerged implementation.

## Historical packages

The October completed-tool and graduation packages retain their recorded results.
Only their pending-tool exclusion is amended here. Do not relabel previous native
probes as proof of reconciliation or mark earlier completed tasks pending again.
