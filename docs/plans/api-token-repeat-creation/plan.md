---
created: 2026-10-27
status: complete
requirements:
  - REQ-AUTH-AUTH-001
system_design:
  - ../../specs/auth/system-design/personal-access-token-creation.md
legacy_specs: []
---

# Implementation plan: Repeat API token creation

## Overview and evidence

A completed first creation succeeds and Done closes the dialog, but reopening
shows the previous raw token instead of the name form. The supplied independent
qualification used the real component, shared controls, providers and English
catalog with only the auth API mocked: one causal failure and one first-create
control passed (native handle 92458, actual exit 1). The dialog remains mounted;
Done calls the parent's `onOpenChange(false)` instead of the dialog's reset
wrapper. Cancel uses the same direct path. Deliver one sequential work order.

## Scope and approach

Route the dialog's explicit and shared close paths through one local reset and
close transition in `apps/web/components/settings/account/api-tokens.tsx`.
Clear name, error, raw token and copied state for a settled close. Retain
existing visual composition, copy, actions, endpoint and list refresh. No API,
backend, schema, policy, other dialogs, or speculative in-flight handling.

Auth owns the behavior (`AC-AUTH-AUTH-001.4`, `.9`, `.10`). Public
`docs/public/authentication.md` is a how-to/reference page and already states
that the token is shown once; this correction makes that supported behavior
work, so no public documentation change is needed. No new copy is planned;
any necessary new copy requires seven catalogs and the i18n gates.

## ASCII UI preview

UI-01: Settings > Account > API Tokens, after Done and New token. Current
evidence: reopening instead shows the previous result. Desktop:

```text
+------------------------------+
| Create API token          X  |
| Description                  |
| Name                         |
| [                         ]  |
| [Cancel]      [Create token] |
+------------------------------+
```

Phone: same focused dialog with the existing stacked footer (primary action
above Cancel):

```text
+------------------------+
| Create API token    X  |
| Description            |
| Name                   |
| [                   ]  |
| [Create token]         |
| [Cancel]               |
+------------------------+
```

Control order and a blank form are required; spacing is illustrative. The
existing constrained centered surface, footer, focus and dismissal behavior
are retained. No responsive or touch geometry is changed. The phone flow
uses taps and asserts visible form and new result without assuming a changed
hitbox. Tests map to `AC-AUTH-AUTH-001.4`, `.9`, `.10`.

## Verification strategy

Author an independent real-rendered `api-tokens.test.tsx`, using real shared
controls/providers/catalog and mocking only the natural auth API boundary.
First run RED on the repeat-create assertion, confirming actual collection and
failure due to the stale result, plus first-create/Done control. GREEN must
prove a second distinct named mint and distinct result, absence of the first
secret on reopening, fresh name and copied indication, and settled Cancel,
close control, Escape and outside-dismissal controls with meaningful input or
result state. Component mocks use dummy tokens. Focused Playwright tests in the auth and
mobile-chrome projects use an isolated auth-enabled backend and exercise the
real route and two successive mints; verify fresh form and distinct result,
with phone taps and no page overflow. Their isolated backend mints disposable
fixture credentials. Compare secrets through a boolean assertion without logging
the values; retain honest runner artifacts and cleanup receipts. These are
browser flow evidence, not a
claim of new geometry or a full UI sizing audit.

## Work orders

- [x] [Task 01: Reset the token creation dialog](task-01-reset-token-dialog.md)

## Verification results

Completed. The independent component regression failed before the fix on the
stale result and retained Cancel name, with the first-create/Done control
passing. After the correction, seven component tests passed. The desktop auth
and mobile-chrome Playwright flows each failed on the absent reopened form
before the fix and passed after it; both include outside dismissal. See the
[work order](task-01-reset-token-dialog.md#results) for commands and evidence.

## Risks

Bypassing the shared close handler can retain the raw token; resetting on only
Done leaves other close routes inconsistent. Real-browser auth fixtures must
restart their isolated backend and restore it. Delayed in-flight responses are
outside this settled-action scope.
