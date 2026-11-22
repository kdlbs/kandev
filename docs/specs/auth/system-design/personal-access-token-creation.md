---
status: current
system: auth
requirements:
  - REQ-AUTH-AUTH-001
---

# Personal access token creation

## Boundary and mapping

Auth owns personal access tokens and the one-time secret contract in
[auth requirements](../requirements/auth.md). This design covers the account
creation interaction, not provider credentials, token generation, authorization,
API schema, or persistence. `AC-AUTH-AUTH-001.4`, `.9`, and `.10` map to the
creation lifecycle below.

## Creation lifecycle

`ApiTokens` owns whether the `MintTokenDialog` is open and refreshes the list
after a successful `mintToken` call from `auth-api`. The mounted dialog owns
the name, error, raw token, and copied indicator. A successful response shows
its raw token in the existing one-time result with Copy and Done. A completed
close clears those local values before the next opening; subsequent opening
starts at the blank form. All close routes, including explicit Done and Cancel
and the shared dialog's close, Escape, and outside dismissal, use the same
close/reset transition. Opening alone does not submit a token request. This is
scoped to settled interactions; it does not define new in-flight race handling.

The endpoint, list refresh, error presentation, existing localized text and
shared dialog primitives remain unchanged. The raw token is not carried into a
new creation attempt. No credential is written to the plan, test logs, or public
docs. Component tests use dummy values; isolated browser fixtures mint disposable
tokens from their own backend, not operator credentials.

## Desktop and phone

Both entry points use Settings > Account > API Tokens and the same state and
controls. The existing shared `DialogContent` centers and constrains the short
form, with its footer stacking on narrow screens. The nearest account phone
example is `mobile-account-security-hostnames.spec.ts`; the shared dialog is
the current surface. This state-only correction adds no geometry, touch target,
navigation, scrolling, or breakpoint changes. Keep the existing surface and
controls; do not claim that its existing hit targets meet a new sizing audit.
A focused desktop and phone browser flow proves repeat creation and visibility;
the component regression proves the other close paths. Any discovered geometry
defect is separate from this repair.
