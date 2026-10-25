---
id: "03-frontend-copy-move-dialog"
title: "Frontend copy/move dialog and API"
status: done
wave: 3
depends_on: ["01-backend-copy-move-service", "02-backend-copy-move-handlers"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/secret-scope-transfer.md"
---

# Task 03: Frontend Copy/Move Dialog and API

## Acceptance

- `copySecret`/`moveSecret` API helpers and `CopyMoveSecretRequest` type exist
  and are covered by API tests; the API layer surfaces the HTTP status so the
  dialog can tell `409` (name field) from other errors.
- `secrets-settings.tsx` is reduced below the 600-line lint limit by extracting
  `SecretListItemRow`, `SecretForm`, and the delete dialog into focused
  components BEFORE the new wiring is added.
- Every secret row on the Global and Workspace secrets pages has a Copy/Move
  button opening one shared dialog with: Copy/Move radio (default Copy),
  destination picker (General + workspaces, excluding the source's own scope,
  and excluding General for a Global source), and an editable name field
  pre-filled with the translated pattern `<name> (from <origin>)` where the
  origin value is the literal `general` for Global sources and the raw
  workspace name for Workspace sources (locale-independent persisted names).
- The dialog handles workspace-list hydration failure explicitly with a real
  retry path and a single source of truth: the picker reads the workspace
  store only, and a dedicated hook exposes `{ loading, error, retry }`
  (no local workspace list in its result). When the store's workspace list is
  empty or hydration failed, the hook fetches `listWorkspaces` itself and
  writes successful results into the store via `setWorkspaces` (with
  request-generation/abort protection so a stale response never overwrites a
  newer hydration). Failures are preserved as `error`, never converted to
  `items: []`. The dialog shows a localized retry/failure state instead of an
  empty picker (a Global source would otherwise have zero valid destinations),
  submit is disabled until a valid destination is available, and the Workspace
  page case is covered (current workspace known, other-workspace list failed).
- A successful-but-empty workspace list is distinct from a failure: the dialog
  shows a localized "no valid destinations" state (never a bare empty picker)
  and keeps submit disabled; a component test covers a Global source with
  `listWorkspaces() === []`.
- Mobile presentation is implemented, not just specified: a responsive
  presentation (Drawer/sheet at phone width or explicit `DialogContent`
  overrides) with safe-area padding, 44px touch targets for the radios,
  destination picker, name input, actions, AND the dialog close affordance
  (the shared `DialogContent` close button is a 24px `icon-sm` and must be
  overridden, e.g. `min-h-11 min-w-11`), and focus restoration on close. The
  scroll invariant is one owner: the Settings content area stays the only
  vertical scroller and the dialog fits without nested vertical scrolling
  (the picker list scrolls only when it cannot fit).
- The default name is truncated so its UTF-8 byte length is at most 100
  (code-point aware), and the copy succeeds for multibyte source names.
- The request payload is a discriminated union: selecting General always
  clears `target_workspace_id`, so a Workspace → General destination switch
  never submits a stale workspace ID (the backend rejects `global` with a
  workspace ID).
- Move mode shows the "original will be removed" warning. A duplicate target
  name blocks submit with the name field marked invalid (red + `aria-invalid`
  + inline error); backend `409` also surfaces on the name field; other
  backend errors show a generic message.
- Submission is guarded against double-clicks: the submit handler checks an
  `isBusy` flag and the primary button is `disabled={isBusy || ...}` while a
  transfer is in flight; a deferred-response test asserts the button stays
  disabled and a second click does not issue another copy/move request.
- A `404` from the backend is deliberately ambiguous (missing source OR
  missing/unauthorized destination) and never discloses which: the dialog shows
  a localized generic failure message and stays open; no rows are removed
  automatically on a `404`, and no destination item is added.
- On success `onCompleted` routes **by the returned item's scope**: a
  Global-target result is added to the Global store from any page; a
  Workspace-target result joins the page's list only when the page is that
  workspace's page; a Move removes the source from the page's own list.
- All new copy is localized in en, pseudo, pt-pt, and zh-cn, including the
  destination-list loading state, the workspace-list failure state, the retry
  action, and the submit-disabled hint; `i18n:check` and `i18n:ratchet` pass.
- Accessibility is explicit, not placeholder-dependent: the dialog has a
  `DialogTitle`, the name and destination controls have real `Label`/`htmlFor`
  associations, the Copy/Move radios form a labeled radio group
  (`fieldset`/`legend` or `aria-label`), conflict errors are associated via
  `aria-describedby`, and the radios are keyboard operable
  (Tab/Arrow/Space). A component test exercises the keyboard path.

## Files likely touched

- `apps/web/lib/types/http-secrets.ts`
- `apps/web/lib/api/domains/secrets-api.ts` (+ `secrets-api.test.ts`)
- `apps/web/hooks/domains/settings/use-secret-destination-names.ts` (new, with
  test)
- `apps/web/hooks/domains/settings/use-workspace-destinations.ts` (new, with
  test; workspace-list loading/error/retry via `listWorkspaces`)
- `apps/web/components/settings/copy-move-secret-dialog.tsx` (new, with test)
- `apps/web/components/settings/secrets-settings.tsx` (extraction + row button
  + dialog state + `onCompleted` routing; extend `secrets-settings.test.ts`)
- Possibly new extracted component files (e.g.
  `secrets-list-item-row.tsx`, `secret-form.tsx`)
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json`

## Inputs

- Tasks 01-02 contracts (request/response shapes, `409`/`404`/`400`/`500`
  semantics).
- Existing `DeleteSecretDialog`/`SecretsSettings` composition, `useSecrets`
  scope routing (`addSecret`/`removeSecret`), store workspaces
  (`workspaces.items` with `id`+`name`), and `useRequest` error handling.

## Dependencies

Tasks 01 and 02.

## TDD sequence

1. Failing API/hook tests for the new helpers, status surfacing, and
   destination-name loading + conflict matching.
2. Failing component tests for dialog rendering, destination filtering,
   default-name construction (literal origin tokens) and byte-aware
   truncation (ASCII, multibyte, and astral-plane surrogate pairs such as
   `😀`/`🔐` — asserting no unpaired surrogates or replacement characters and
   `new TextEncoder().encode(result).length <= 100`), conflict blocking, move
   warning, `409` surfacing, destination switching Workspace → General
   asserting the payload carries no workspace target, a serialized-payload
   test asserting `name: null` is never emitted, workspace-hydration-failure
   rendering a retry/error state with submit disabled (Global source) and
   `retry` re-fetching workspaces, mobile presentation (phone-width sheet,
   44px targets, no overflow), keyboard operation of the Copy/Move radios,
   the primary submit label switching between Copy and Move modes (toggling
   the radio changes the button label), an in-flight duplicate-click test
   (deferred response: button disabled, no second request), an ambiguous
   `404` test (generic dialog feedback, dialog stays open, no row removal, no
   destination item added), and `onCompleted` routing in both directions
   (workspace page →
   Global target lands in the store; Global page → workspace Move removes the
   source from the Global list and routes the returned workspace item only
   when the page is that workspace).
3. Extract the oversized sections from `secrets-settings.tsx` (existing tests
   must stay green), then implement the dialog, row button, wiring, and i18n
   keys; run focused frontend tests, typecheck, lint, and i18n checks.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Risks

- Route the `onCompleted` update by the returned item's scope, never the page
  scope: a Workspace page move/copy to General must add to the Global store.
- The destination-name pre-check is best-effort; the backend `409` must still
  be handled and shown on the name field.
- Persisted secret names are user data and must never be routed through a
  catalog key; only the `{{name}} (from {{origin}})` pattern is translated,
  and origin values stay literal.
- Truncation must be UTF-8 byte-aware to match the Go `len()` rule; do not use
  `String.prototype.length` for the limit.
- No em dashes in user-facing copy; keep icon-action accessible labels.
- Keep `secrets-settings.tsx` under 600 lines after all changes.

## Output contract

Report the new/changed components, hooks, extraction result, keys, test names,
and the verification output.
