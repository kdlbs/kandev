---
spec: docs/specs/plugins/voice-extraction-host.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Voice Plugin Host Prerequisites

## Overview

Add host-owned composer adapters to every Voice Mode surface, then extend existing webhooks with
per-declaration access and request-size policies. Publish the contract through frontend types,
manifest validation, the frozen plugin API, and public authoring documentation.
Keep core Voice Mode installed throughout; a separate plugin repository will consume these APIs and
only a later parity task may remove core code.

## Contract Decisions

- Composer state stays local to each mounted surface. Slot props expose current disabled/submittable
  state plus insertion, focus, and native submit without exposing draft contents or private refs.
- `chat-input-actions` grows compatible props and covers task chat plus Quick Chat. Creation forms get
  explicit `task-create-input-actions` and `new-session-input-actions` slots because their identifiers,
  validation, and submit lifecycles differ.
- Plugin UI continues to use `host.api.fetch("webhooks/<key>")` and the existing `HandleWebhook` RPC.
  `webhooks[].access` selects public or authenticated invocation and `max_body_bytes` sets a bounded
  per-key limit up to 16 MiB. API v1 manifests retain public/4 MiB defaults. API v2 defaults to
  authenticated access and a 4 MiB limit.
- Plugin localization is a separate prerequisite. It must be resolved before core Voice Mode is
  removed, but this package adds no host i18n API.

## Frontend Composer Architecture

- Add public types in `apps/web/lib/plugins/types.ts` and mirror them in
  `docs/plans/plugins/PLUGIN-API.md`. Implement adapters guarded by the mounted composer generation;
  normal React props update disabled/submittable presentation without draft state entering Zustand.
- Extend `TipTapInputHandle` only where required for the host adapter. Reuse its selection-aware
  `insertText`, but centralize the Voice spacing rule so textarea and TipTap adapters behave identically.
- Adapt `useChatInputState`/`ChatInputArea` to expose native `handleSubmit` and the same disabled gate
  to `ChatInputPluginActions`. Do not call `useMessageHandler` or construct `ChatSubmitPayload` from
  plugin code. Quick Chat inherits the adapter through its existing `ChatInputArea` composition.
- Extend `TaskFormInputsHandle` with guarded focus/insertion operations or provide a sibling adapter
  around `useDescriptionInput`. Task creation invokes its existing form submit handler; new-session
  creation invokes its existing `handleSubmit`, including profile, upload, and busy-state gates.
- Keep slot controls in the existing toolbars on desktop. On native mobile, use the existing composer
  toolbar/overflow composition and 44px touch targets; do not introduce a separate drawer solely for
  plugin actions.

## Mobile Design Contract

- **Outcome and entry points:** Voice plugin actions remain beside native composer controls in task
  chat and inside the prompt toolbar for task/new-session creation; Quick Chat uses its existing input
  toolbar. All actions are reachable by touch.
- **Exemplars:** task chat reuses `chat-input-toolbar-mobile.tsx` and
  `mobile-voice-mode.spec.ts`; new session reuses the full-height dialog in
  `mobile-new-session-dialog.spec.ts`; task creation reuses the existing responsive Task Create dialog.
- **Hierarchy and action:** the plugin action is secondary to the native submit action and never hides
  submit, model/profile selection, or cancellation. Recording/transcription state belongs
  to the plugin control; the native composer remains the single draft and submit owner.
- **Surface rationale:** frequent, one-tap input augmentation belongs inline in the existing toolbar,
  not behind navigation or a new drawer. Existing dialog and composer scroll owners remain unchanged;
  overlays and toolbars keep safe-area clearance and no document horizontal overflow.
- **Shared behavior:** capability state and native adapters are shared across presentations. Only the
  existing toolbar placement differs. Pixel 5 Playwright proves insertion plus native submission in
  task chat, task creation, and new-session creation; Quick Chat mobile coverage is included if its
  shipped mobile entry exposes the full toolbar.

## Backend Authenticated Webhook Architecture

- Extend manifest `Webhook` with `Access` and `MaxBodyBytes`. Validate `access` as
  `public|authenticated`, use the API v1 public default and API v2 authenticated default, default the
  limit to 4 MiB, reject non-positive or >16 MiB values, and preserve those values through
  install/registry serialization.
- Keep the existing webhook URL and `HandleWebhook` RPC. In the handler, resolve the declared key
  before reading the body, apply its effective limit, and require normal Kandev identity plus accepted
  Origin for authenticated cookie requests. Authentication-disabled mode uses the synthetic user.
- Propagate `Request.Context()` cancellation through `Service.InvokeWebhook`; bound and sanitize the
  response as today. Logs may add access mode and byte count but never request content or secrets.
- Keep existing public webhooks byte-for-byte compatible: no authentication requirement and a 4 MiB
  default. The Voice plugin stores its OpenAI key through existing secret config fields; no host key
  storage or new secret API is introduced.

## Localization And Compatibility

- Keep API v1 omissions public and introduce API v2 with an authenticated default. Document that
  consumers must declare the first supporting `min_kandev_version`; older hosts fail installation
  instead of falling back to webhooks.
- Update `docs/specs/plugins/spec.md`, `docs/plans/plugins/PLUGIN-API.md`,
  `docs/public/plugins-authoring.md`, and `docs/public/plugins-manifest.md` in the same contract task.

## Test Strategy

- Frontend unit/component tests prove selection replacement, spacing, focus, live state props,
  stale-adapter failure, native submit delegation, blocked submission, cleanup, Quick Chat props, and
  desktop/mobile slot placement across all composer families.
- Backend manifest tests prove defaults, validation, and compatibility. Handler/service tests prove
  authenticated/public access, Origin enforcement, 10 MiB success, >16 MiB rejection, cancellation,
  disabled/runtime failure, and unchanged legacy webhook behavior.
- Desktop and mobile Playwright install or inject a fixture plugin action, insert a distinctive string
  at a non-terminal selection, invoke native submit, and assert the resulting task/message/session.
  A backend-integrated E2E action uploads >10 MiB only where the container/runtime fixture is available;
  focused Go integration tests remain the deterministic size/security proof.

## Implementation Waves And Tasks

Wave 1:

- [x] [Task 01: Publish composer contract](task-01-publish-composer-contract.md)
- [x] [Task 02: Extend webhook declarations](task-02-authenticated-webhook-manifest.md)

Wave 2:

- [x] [Task 03: Adapt chat and Quick Chat composers](task-03-chat-composer-adapters.md) (depends on Task 01)
- [x] [Task 04: Adapt creation composers](task-04-creation-composer-adapters.md) (depends on Task 01)
- [x] [Task 05: Enforce authenticated webhooks](task-05-authenticated-webhook-handler.md) (depends on Task 02)

Wave 3:

- [x] [Task 06: Document and verify extraction prerequisites](task-06-docs-e2e-verification.md) (depends on Tasks 03-05)

Tasks in the same wave are parallel-safe only where their owned files remain disjoint. Execution stays
sequential unless the user explicitly authorizes subagents.

## Risks

- A capability that snapshots React state can auto-submit after a gate changes; `submit()` must
  revalidate the native gate at call time.
- React Strict Mode or a task/session switch can leave stale plugin closures alive; generation guards
  must make them fail closed rather than target the newly mounted editor.
- Conditional authentication inside a route currently treated as public must fail closed per declared
  key; direct handler and middleware-boundary tests are required.
- Raising gRPC message limits globally would unnecessarily affect other plugin traffic; configure only
  the existing webhook call path sufficiently for the 16 MiB host ceiling.
- Core Voice removal in this package would make rollback and parity validation unsafe; it is explicitly
  deferred to the plugin implementation/extraction task.

## Verification Commands

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins components/task/chat components/task-create-dialog-selectors.test.tsx components/task/new-session-dialog.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
make build-web && make build-backend
cd apps/web && pnpm e2e:raw --project=chrome e2e/tests/plugins/composer-actions.spec.ts e2e/tests/chat/quick-chat.spec.ts e2e/tests/session/new-session-dialog.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/plugins/mobile-composer-actions.spec.ts e2e/tests/session/mobile-new-session-dialog.spec.ts
node --test scripts/validate-public-docs.test.mjs
```
