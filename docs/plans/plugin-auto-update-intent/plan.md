---
created: 2026-10-21
status: done
requirements:
  - REQ-PLUGINS-MARKETPLACE-004
system_design:
  - ../../specs/plugins/system-design/marketplace.md
legacy_specs: []
---

# Implementation Plan: Plugin auto-update intent

## Overview

Repair the mounted immediate preference flow in one sequential work order.
Independent PUTs currently overwrite newer choices with old receipts and roll
back to unaccepted optimistic snapshots. Parent synthetic HTTP diagnostics
proved both stale success and double-failure behavior at base
`9bfb7da156b25155626ead19f8eceaff9b1ea4e0`.

## Scope

Own global preference orchestration, inherited row truth, and regression proof.
Exclude backend/API/schema changes, per-plugin mutations, worker policy,
installation policy, cross-tab/remount ordering and ambiguous transport guarantees.

## Technical approach

Keep `useAutoUpdateSettings` as page owner. Serialize each accepted intent with
one promise tail, capture identity synchronously, preserve the latest optimistic
value, and reconcile against successful GET/PUT data. Failed initial GET keeps
an explicitly unknown baseline with the existing off fallback. Drain accepted
writes after disposal without UI/toasts; reject stale disposed callbacks.
A local queue follows existing immediate-setting serialization patterns without
introducing a shared framework or moving this preference into Save/Discard.
The existing marketplace pair owns this capability; no ADR is needed for this
local orchestration choice. Public documentation's opt-in/override instructions
remain accurate; no labels, layout or public API change.

## ASCII UI preview

UI-01: Settings > Plugins > Installed, shared desktop/phone composition:

```text
Automatically update plugins       [on/off]
Installed plugin (inherits)        [on/off]
Installed plugin (override)        [on/off] Reset
```

The existing phone settings row and mobile plugin row remain the exemplars.
The page retains its scroll owner and existing touch targets. Structural
requirement: pending latest choice appears in global/inherited switches while
explicit overrides stay fixed; failures restore accepted data. Spacing is
illustrative. Loading still disables the global switch only until GET settles.

## Tests

AC-PLUGINS-MARKETPLACE-004.1-.3: real-hook/API transport tests in
`hooks/domains/plugins/use-auto-update-settings.test.ts`, plus real page/row
consumer coverage in `app/settings/plugins/page-auto-update.test.tsx`.
Cover ordered successes, alternating intents, mixed/all failures, normalized
receipts, same-tick callbacks, failed GET recovery and unmount ownership.
Existing inert preference mocks do not prove these outcomes.

## E2E tests

AC-PLUGINS-MARKETPLACE-004.1-.3: desktop and phone switch actions with held
backend receipts, actual persisted readback/reload, and synthetic HTTP failures.
Use disposable managed backend plus the local fixture, no eligible external
catalog updates. Restore original global preference and verify readback in
finally. Specs: `plugin-auto-update.spec.ts` and
`mobile-plugin-auto-update.spec.ts`, sharing `plugin-auto-update-helpers.ts`.

## Work orders

- [x] [Task 01: Preserve immediate preference intent](task-01-preserve-intent.md)

## Verification results

[Task 01](task-01-preserve-intent.md#results) records causal baseline failures
and final results: 37 focused tests, 2 desktop and 2 phone cases, scoped ESLint,
Node 24 typecheck under the shared 8 GiB guard, catalog validation, all 36
specification-linter tests, full specification lint, documentation coverage
preflight and whitespace checks passed. Successful browser writes were read
back from the disposable backend; failures were synthetic HTTP 503 responses.
Original preference restoration and local fixture removal run in finally.
Normal-hook commit and exact-head evidence are reported at the parent-validation
checkpoint. Public documentation's existing instructions remain accurate.

## Risks

Accepted writes may finish after navigation; a new page has independent ownership.
A response loss can leave persistence uncertain; no client-only exactly-once
claim is made. Initial read failure leaves the compatibility fallback unconfirmed.
No push/PR before separate parent authorization after normal-hook commit and
actual-diff parent validation.
