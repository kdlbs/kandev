---
created: 2026-10-22
status: done
requirements:
  - REQ-INTEGRATIONS-NATIVE-CREATE-001
system_design:
  - ../../specs/integrations/system-design/native-change-request-creation.md
legacy_specs: []
---

# Implementation Plan: Change-Request Dialog Ownership

## Overview

One sequential work order fences current-form mutation after native creation.
The caller explicitly authorizes this package followed by same-session TDD.
Stop after normal hooks, a clean local commit, checks and resource closure for
parent validation. Push/PR require a separate later delivery grant.

## Scope and confirmed evidence

The preserved real-provider/WS diagnostic at immutable base
`8ba5b6909139ec2e9e14b0e2eb2f96a4e342e88b` has two causal failures: stale success
clears a reopened raw draft; stale partial failure leaks pushed state to another
repository. The owned partial-failure control passes. The handler's unconditional
setters are the cause. No backend data loss or actual remote duplicate is proven.

Integrations owns this contract because retry state depends on the provider's
successful push. Existing GitLab requirements are provider-specific. The
existing provider mutation ADR supplies the architecture; no new ADR is needed.

## Technical approach

Give each opening an opaque form owner, invalidate across session/task lifetime,
and check ownership before completion setters. Capture original task association
and creation scope. Preserve existing create closure, abort controller, feedback,
immediate-close behavior and owned partial retry. Leave commit state unchanged.

| Route | Transport / identity | Verification / fallback |
| --- | --- | --- |
| Built-in | WS session + repo + base + draft | Real state/API/transport integration; existing fallback unchanged |
| Registered provider | Captured target + push then callback | Registered provider consumer test; owned retry skips push |
| Phone | Shared native dialog via Changes | Pixel 5 touch emulation with disposable backend |

No SDK/backend/cache/navigation/utility-generator redesign, unrelated integrations,
Kanban changes, worker sessions, push, or PR is in scope.

## UI-01: Shared create dialog

Desktop entry: Changes repository create control or Git menu. Phone entry:
bottom navigation Changes, then repository create control. Existing composition
is retained. Both use the same host-owned state.

```text
Create Pull Request (repository)
Title       [current raw title       ]
Description [current raw description ]
[x] Create as draft
[Cancel]                    [Create PR]
```

Before: older completion clears this reopened form or adds Retry state.
After: this reopened form keeps text, selection and actions; the old outcome is
reported separately. An owned partial failure instead reopens the original form
with its partial-push notice and Retry action. Diagram spacing is illustrative.
No layout/touch-size changes are planned; the existing dialog owns scrolling.
Maps AC-INTEGRATIONS-NATIVE-CREATE-001.2, .4 and .5 to the rendered tests.

## Tests and E2E

`components/vcs/vcs-dialogs.change-request.test.tsx` exercises real state, dialog,
Git hooks and deferred WS transport for .1-.4, plus registered provider retry.
`e2e/tests/git/change-request-ownership.spec.ts` and its `mobile-` counterpart
exercise shipped desktop/phone entry and controlled causal completion against
an isolated backend/offline repository and mock provider for .2/.5.

## Work orders

- [x] [Task 01: Preserve current request](task-01-preserve-current-request.md)

## Verification results

Package created before permanent tests/production edits. The work order records
63 passing focused tests after bounded task-identity review remediation, passing rebuilt desktop and Pixel 5 touch scenarios,
static/i18n/docs checks, causal RED and controls, and cleanup. External receipts
retain exact logs, package timestamps and the full commit diff. Local completion
stops at parent validation, without push or PR. Numeric instrumented coverage
was not collected because its optional Vitest provider is not installed.

## Risks

Provider responses can outlive the form; ownership must not suppress truthful
outcomes. Equal values and away/back identities require generation identity,
not value equality. Browser fixtures must never mutate worker seed/operator data.
