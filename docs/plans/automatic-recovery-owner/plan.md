---
created: 2026-09-29
status: done
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Automatic recovery presentation ownership

## Overview

Repair automatic recovery feedback escaping the composer ownership rule. One
sequential work order integrates the page, composer and preview paths, then
proves desktop and phone behavior. This is conformance to existing requirement
006, especially criteria .1, .2, .5, .11, .16, .17 and .19; no new requirement,
backend contract or architecture decision is needed.

## Evidence and root cause

Source inspected at 320f050e1. The supplied screenshot shows the red "Session
recovery failed" page banner and amber composer card simultaneously, plus a
workspace pane linking to recovery. The screenshot does not establish why the
underlying recovery failed or the deployed commit.

`TaskPageRecoveryFeedback` in `task-page-inner.tsx` suppresses the page fallback
only when `resolveTaskPageBootstrapRecoveryError` returns a bootstrap error.
`selectSessionRecoveryError` requires `phase === "bootstrap"`.
In contrast, `selectActiveSessionRecovery` can return a generic composer model
for FAILED sessions with no structured bootstrap metadata. Thus the same view
can render `SessionRecoveryFeedback` above tabs and `SessionRecoveryCard` in the
composer. This is a deterministic source trace, not a completed browser repro.

Minimal fixture: selected non-passthrough FAILED session, no bootstrap metadata
or bootstrap active_error, automatic resumption error and recovery_failed result
with separate resume/restore causes. The page takes its legacy banner branch;
the composer selector returns a generic model. Also cover an explicit current
non-bootstrap error and WAITING_FOR_INPUT with error_message.

Simply hiding the banner loses information: `matchingAutomaticRecovery` in
`session-recovery-model.ts` requires an aggregate active_error session match,
and `buildBootstrapRecoveryModel` consumes automatic causes only for bootstrap
models. The fix must transfer request-local causes and attempt ownership before
suppressing the outer surface. `PreviewSessionRecoverySurface` has the same
legacy feedback component and needs equivalent coverage.

## Scope

In scope: ownership selection, automatic feedback delivery, mobile offset,
preview parity, focused component and browser regressions.

Out of scope: determining why this user's workspace or queued launch failed,
backend recovery policy, persistence migrations, clearing user errors, new
recovery operations, provider-specific behavior changes and visual redesign.

## Technical approach

Follow the existing [system design](../../specs/agents/system-design/session-recovery-failures.md),
Recovery presentation ownership and Uniform active recovery presentation, plus
[ADR September 20](../../decisions/2026-09-20-active-session-recovery-owner.md).

1. Derive page/composer ownership from the same active recovery selection and
   actual chat/fallback capability, rather than bootstrap phase alone. Pass
   explicit task/session and current recovery attempt identity through the
   existing TaskLaunchErrorProvider. Do not infer correlation from matching text
   or a shared session ID alone. Preserve navigation-generation guards.
2. Deliver current automatic resume/restore results to that owner even without
   durable bootstrap metadata. Preserve separately labelled sanitized causes,
   shared busy admission and existing eligible actions. Distinct/unmatched
   causes remain visible; they must not be silently absorbed or authorize retry.
3. Suppress the page duplicate only after that owner can represent the current
   feedback. Use the same decision for phone top clearance. Retain explicit
   fallback for no-session/passthrough surfaces and status-only uncertainty.
   Audit preview and Quick Chat consumers without broadening their permissions.
4. Preserve independent shared task errors, historical errors and workspace
   navigation to the composer. No second mutable error store or recovery hook.

## Preview Plan correction

The first PR review remediation restored the legacy banner above the agent tabs
when Plan replaced Chat. That preserved access but violated the shared-card
placement contract. Plan must retain the shared `SessionRecoveryCard` below its
content, at the composer location. Mount the same selection/pending provider and
manual recovery actions only while Plan owns that region. Put remaining
session-specific fallback feedback below content too; independent task errors
retain their existing shared strip.

```text
Task header
Agent / Plan tabs
Plan content (scrolls)
[Session recovery failed]
[Resume session] [Start fresh session]
[Technical details]
```

The browser regression must assert the shared card, absence of the upper legacy
banner, placement below tabs and at the panel bottom, both automatic causes,
manual pending guards and a return to Chat with one owner. Phone task cards open
the full task page instead of preview; its existing recovery scenario remains
the phone coverage. Capture the corrected preview before updating the PR image.

## ASCII UI preview

### UI-01: Task Chat after automatic recovery fails

Before (screenshot and source): task header, red recovery banner, tabs,
conversation, amber composer recovery card.

After, desktop:

```text
Task header
Session / Plan / Pull request tabs
Conversation history                 Files: Workspace unavailable
                                     [View recovery]
[Session recovery failed]
[Resume session] [Start fresh session]
[Technical details: resume / restore]
```

After, phone:

```text
Task header / session picker
Conversation history
[Session recovery failed]
[Resume session               ]
[Start fresh session          ]
[Technical details            ]
Chat / Files / More
```

One active owner and operation-labelled diagnostics are structural requirements;
copy and spacing are illustrative. Keep existing capability-driven actions. The
card replaces the blocked composer, with its existing bounded recovery-region
scroll owner; history remains independently scrollable. Details have no nested
scroller. Phone actions stack with 44px targets and existing safe-area clearance.
Desktop actions wrap with 28px targets. Pending keeps this owner and disables
equivalent actions; successful agent recovery restores the composer; workspace
restoration alone retains stopped-agent feedback. Independent task errors retain
their shared strip. No-session and passthrough cases retain an explicit fallback.

## Mobile design contract

Reuse `SessionRecoveryCard`, `RecoveryActions`, and task-layout's dedicated phone
composition as the shipped exemplars. Recovery stays inline because it replaces
messaging; no modal or drawer is needed. Share ownership and actions across
viewports. Preserve dynamic viewport, safe-area wrappers and existing recovery
scroll bounds. View recovery from Files activates Chat and focuses the owner.
Verify no empty header gap remains after removing the banner.

## Tests

Add `task-page-recovery-feedback.test.tsx` with a composed page/composer fixture:
"automatic recovery without bootstrap metadata has one active owner" must fail
before correction. Assert no page banner, exactly one active recovery surface,
and both labelled safe causes. Cover missing and explicit non-bootstrap error,
matching and mismatched attempt identities, pending, stale results, success,
workspace-only success, no-session/passthrough fallback, status unavailable,
independent task error and session switching. Extend existing selector, recovery
card, preview and mobile-offset suites. These cover criteria .1/.2/.5/.11/.16/.17.

## E2E tests

Extend `e2e/helpers/session-error-recovery-ui.ts` through the existing desktop
and mobile spec entry points with "automatic recovery has one owner". Exercise
failed automatic resume plus failed workspace restore without bootstrap metadata,
expand both diagnostics, follow View recovery from Files, retry, and retain
history. Assert one surface, shared pending, no page banner, no duplicate
announcement, no horizontal overflow, phone touch sizes and no leftover top gap.
Include an independent task error that remains visible. Covers .11/.16/.17/.19.
Use causal transport waits and isolated fixtures, not the user's live instance.

## Work orders

- [x] [Task 01: Unify automatic recovery ownership](task-01-ownership.md)

## Verification results

Implementation is present. The task page and preview share composer eligibility;
request-local diagnostics retain task/session/generation/attempt identity and
existing stale-request guards. Correlated workspace failures reveal Chat.

Final verification: 158 targeted unit/component/hook tests passed; typecheck,
full lint, i18n checks and both public-doc validators passed. All nine desktop
and nine phone recovery browser scenarios passed with one worker and no retries.
The mobile capture confirms the planned single-card composition, stacked touch
actions, retained history and no duplicate banner or phantom header clearance.

The automatic-recovery browser fixture uses a launched session so workspace
restoration has a canonical environment; a bare seeded session correctly cannot
dispatch that operation. Browser coverage proves both automatic diagnostics,
Files-to-Chat focus, history retention, manual retry pending and failure feedback.
See the work order for commands and evidence limitations.

## Related packages

[Session error UI](../session-error-recovery-ui/plan.md) and
[error scope/history](../error-scope-and-history/plan.md) are predecessor delivery
records. Their historical passing tests do not cover this automatic fallback
fixture. This package adds the missing regression without changing their accepted
placement, scope, operation or persistence contracts.

## Risks

Suppressing feedback before transferring causes would hide the failure. Inferring
ownership by session alone could hide an independent incident. Removing every
outer surface would strand passthrough/no-composer users. Mobile padding and
workspace reveal targets must follow the same ownership decision.
