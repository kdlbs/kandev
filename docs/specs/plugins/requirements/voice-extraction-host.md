---
status: active
system: plugins
created: 2026-08-11
owners:
  - kandev
---
# Voice Plugin Host Prerequisites Requirements

## Overview

Voice Mode can ship independently only if a plugin can participate in every native prompt composer without bypassing its draft and submission behavior, and can invoke its own billable backend without exposing that operation as a public webhook. The host must provide those narrow capabilities before the existing core implementation can move to a dedicated plugin repository.

## Requirements

### REQ-PLUGINS-VOICE-EXTRACTION-HOST-001: Voice Plugin Host Prerequisites

**Intent:** Voice Mode can ship independently only if a plugin can participate in every native prompt composer without bypassing its draft and submission behavior, and can invoke its own billable backend without exposing that operation as a public webhook. The host must provide those narrow capabilities before the existing core implementation can move to a dedicated plugin repository.

#### Acceptance criteria

- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.1:** A plugin action rendered in a supported composer receives typed composer props owned by that mounted composer. They report whether editing and submission are currently allowed, insert text at the current selection, focuses the editor, and submits through the same handler as the native submit control.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.2:** `composer.insertText(text)` trims an empty transcript to a no-op, replaces the selected range (or inserts at the caret), applies the native Voice Mode word-boundary spacing rule, places the caret after the insertion, updates the authoritative local draft synchronously, and focuses the input.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.3:** `composer.submit()` returns a typed outcome and never constructs or sends a message itself. The native path remains responsible for all existing submission behavior and errors.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.4:** Slot props expose live `disabled`, `submittable`, and optional `disabledReason` state. An operation against an unmounted or superseded composer fails closed with `unavailable`; submit against a currently blocked form returns `blocked` without discarding the draft.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.5:** Composer action slots exist in desktop and native-mobile task chat, Quick Chat, task creation, and new-session creation. Each slot reports its surface and presentation plus only the identifiers meaningful on that surface.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.6:** The existing `chat-input-actions` registration remains source-compatible. Its slot props gain the composer capability and surface metadata; new `task-create-input-actions` and `new-session-input-actions` slots cover creation forms. Quick Chat uses `chat-input-actions` with `surface: "quick-chat"` and `taskId: null` when it is genuinely task-less.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.7:** A webhook declaration chooses `access: public` or `access: authenticated`. Existing declarations default to `public`; authenticated webhooks require the current Kandev user and same-origin browser request checks before the existing plugin webhook RPC runs.
- **AC-PLUGINS-VOICE-EXTRACTION-HOST-001.8:** A webhook declaration may lower its request cap through `max_body_bytes`. Only authenticated webhooks may raise it above the existing 4 MiB public ceiling, up to a host ceiling of 16 MiB. Voice transcription declares an authenticated webhook with a limit of at least 10 MiB.

## Migrated source detail

## Why

Voice Mode can ship independently only if a plugin can participate in every native prompt composer
without bypassing its draft and submission behavior, and can invoke its own billable backend without
exposing that operation as a public webhook. The host must provide those narrow capabilities before
the existing core implementation can move to a dedicated plugin repository.

Decision: [ADR-2026-08-11-composer-access-authenticated-webhooks](../../../decisions/2026-08-11-composer-access-authenticated-webhooks.md).

## What

- A plugin action rendered in a supported composer receives typed composer props owned by that
  mounted composer. They report whether editing and submission are currently allowed, insert
  text at the current selection, focuses the editor, and submits through the same handler as the
  native submit control.
- `composer.insertText(text)` trims an empty transcript to a no-op, replaces the selected range (or
  inserts at the caret), applies the native Voice Mode word-boundary spacing rule, places the caret
  after the insertion, updates the authoritative local draft synchronously, and focuses the input.
- `composer.submit()` returns a typed outcome and never constructs or sends a message itself. The
  native path remains responsible for all existing submission behavior and errors.
- Slot props expose live `disabled`, `submittable`, and optional `disabledReason` state. An
  operation against an unmounted or superseded composer fails closed with `unavailable`; submit
  against a currently blocked form returns `blocked` without discarding the draft.
- Composer action slots exist in desktop and native-mobile task chat, Quick Chat, task creation, and
  new-session creation. Each slot reports its surface and presentation plus only the identifiers
  meaningful on that surface.
- The existing `chat-input-actions` registration remains source-compatible. Its slot props gain the
  composer capability and surface metadata; new `task-create-input-actions` and
  `new-session-input-actions` slots cover creation forms. Quick Chat uses `chat-input-actions` with
  `surface: "quick-chat"` and `taskId: null` when it is genuinely task-less.
- A webhook declaration chooses `access: public` or `access: authenticated`. Existing declarations
  default to `public`; authenticated webhooks require the current Kandev user and same-origin browser
  request checks before the existing plugin webhook RPC runs.
- A webhook declaration may lower its request cap through `max_body_bytes`. Only authenticated
  webhooks may raise it above the existing 4 MiB public ceiling, up to a host ceiling of 16 MiB.
  Voice transcription declares an
  authenticated webhook with a limit of at least 10 MiB.
- The Voice plugin's OpenAI key belongs to its existing plugin settings and secret storage. Kandev
  relays the request but does not own, expose, or select that key.
- Plugin-owned copy uses the scoped catalog and reactive locale API established by
  [ADR-2026-08-12-plugin-localization-contract](../../../decisions/2026-08-12-plugin-localization-contract.md).
  The later Voice plugin registers its English fallback and supported-locale catalogs before any
  composer contribution.
- Disabling, uninstalling, or reloading a plugin aborts its in-flight webhook requests, unregisters
  its composer actions, and leaves native drafts intact.
- The current core Voice Mode stays present until a separately delivered Voice plugin proves parity
  on all listed surfaces. Removing core Voice Mode is not part of these host prerequisites. That
  removal has since happened; see [Voice Mode leaves core](voice-extraction.md).

## API Surface

### Composer slots

```ts
type PluginComposerSurface =
  | "task-chat"
  | "quick-chat"
  | "task-create"
  | "new-session";
type PluginPresentation = "desktop" | "mobile";

type PluginComposerSubmitResult =
  | { status: "submitted" }
  | { status: "blocked"; reason?: string }
  | { status: "unavailable" };

interface PluginComposerCapability {
  insertText(text: string): { status: "inserted" | "ignored" | "unavailable" };
  focus(): { status: "focused" | "unavailable" };
  submit(): Promise<PluginComposerSubmitResult>;
}

interface PluginComposerSlotProps {
  surface: PluginComposerSurface;
  presentation: PluginPresentation;
  taskId: string | null;
  taskTitle?: string;
  activeSessionId: string | null;
  sessionIds: string[];
  disabled: boolean;
  submittable: boolean;
  disabledReason?: string;
  composer: PluginComposerCapability;
}
```

The host re-renders slot props when native state changes. `submit()` always revalidates native state
at call time, reading the live draft rather than the last render's snapshot, so a plugin that inserts
and submits in one callback is not told `blocked` for text it just inserted.

The host replaces the capability object whenever the composer's identity changes: its surface, task
or session. Methods on the superseded object return `unavailable`. Identity matters separately from
mounting because a composer is re-rendered, not remounted, when the user switches task or session, so
without it a handle captured while recording on one conversation would act on the next.

### Authenticated webhooks

Manifest:

```yaml
webhooks:
  - key: transcribe
    access: authenticated
    max_body_bytes: 16777216
```

Frontend:

```ts
host.api.fetch("webhooks/transcribe", {
  method: "POST",
  body: formData,
  signal,
});
```

HTTP and runtime wire contract:

```text
POST /api/plugins/{pluginId}/webhooks/{key}
Origin: <the configured Kandev web origin>

rpc HandleWebhook(WebhookRequest) returns (WebhookResponse)
```

`access` is enforced by the host before `HandleWebhook` runs. For `authenticated`, the normal Kandev
identity must be present, and a session-cookie identity additionally requires a same-origin request.
Exactly two signals accept it: an accepted Origin, or (when the browser sends no Origin, as it does
not on same-origin GET/HEAD) a `Sec-Fetch-Site` of `same-origin` or `none`. Everything else is
rejected, including `cross-site`, `same-site`, and a request carrying neither header. PAT and
synthetic identities are not ambient and are not subject to that check;
authentication-disabled installations use the synthetic default user. Public webhooks retain their
current external-call behavior. Request cancellation propagates through the existing RPC context.

## Permissions

- Composer capabilities exist only as props on a currently rendered slot; they are not placed in
  `host.store`, exported globally, or addressable by task/session ID.
- Webhook access changes only who may invoke that declared key. It grants no Host data, state, secret,
  or event capability.
- Plugin backends receive the existing webhook request shape. They never receive the Kandev session
  cookie or PAT. The Voice plugin reads its OpenAI secret through existing plugin configuration.

## Failure Modes

- Empty or whitespace-only insertion is ignored. Unsupported or stale composers return
  `unavailable`; they do not mutate another surface's draft.
- A blocked native submit returns `blocked`, preserves the native draft, and leaves native error
  presentation authoritative. A native submission failure resolves `blocked` with its safe reason
  where available and preserves the draft exactly as a native failure does.
- An unavailable plugin runtime returns `503`; cancellation stops the runtime request and returns no
  synthetic success; an oversized body returns `413`; malformed multipart or metadata returns `400`;
  missing authentication returns `401`; origin or declaration failure returns
  `403` or `404` according to the existing hide-existence policy.
- Host logs may contain plugin ID, action, request ID, byte count, duration, status, and cancellation,
  but never audio bytes, authorization material, cookies, or plugin secrets.

## Compatibility And Versioning

- These additions are backward-compatible within plugin `api_version: 1`; all new manifest and host
  fields are optional, and existing slot components that ignore added props continue to work.
- A plugin using `webhooks[].access` or `max_body_bytes` must set `min_kandev_version` to the first
  release containing this contract. Older hosts reject installation instead of silently treating an
  authenticated webhook as public.
- Manifest types, validation, authoring docs, and handler behavior change together. The protobuf and
  Go plugin SDK webhook contract remain unchanged.

## Scenarios

- **GIVEN** a task-chat selection, **WHEN** a plugin inserts a transcript and invokes submit, **THEN**
  the selection is replaced and the existing native submit handler runs.
- **GIVEN** a running agent configured for steering or queueing, **WHEN** a plugin auto-submits,
  **THEN** the same steering or queueing decision and error handling as the native submit button runs.
- **GIVEN** task creation or new-session creation on desktop or Pixel 5, **WHEN** a plugin inserts at
  the textarea selection and submits, **THEN** the native form validation and creation handler run.
- **GIVEN** a plugin action is recording on one session, **WHEN** the task, session, or composer
  changes, **THEN** the plugin cancels its request and the stale capability cannot modify the new
  composer.
- **GIVEN** Quick Chat, **WHEN** a composer plugin action is rendered, **THEN** it receives the shared
  capability with `surface: "quick-chat"` and completes the same insertion and submit behavior.
- **GIVEN** a plugin declares an authenticated `transcribe` webhook with a 16 MiB cap, **WHEN** its UI
  sends 10 MiB of multipart audio through `host.api.fetch`, **THEN** the existing webhook RPC reaches
  that plugin backend and returns its response.
- **GIVEN** an unauthenticated caller, **WHEN** it calls the authenticated transcription webhook,
  **THEN** the request fails before plugin execution and cannot consume the plugin's configured key.
- **GIVEN** an in-flight transcription, **WHEN** the caller aborts or the plugin is disabled, **THEN**
  the HTTP and runtime requests are cancelled and no transcript is inserted.
- **GIVEN** a plugin built for this contract, **WHEN** it is installed on an older Kandev release,
  **THEN** `min_kandev_version` rejects installation with no partial registration.

## Out Of Scope

- Implementing, packaging, publishing, or removing the dedicated Voice plugin.
- Removing core Voice Mode before plugin parity and migration are separately validated.
- Exposing draft text, selection offsets, or arbitrary native composer mutation through
  `host.store` or a global host API.
- General authenticated REST routing, response streaming, bidirectional browser sockets, or changing
  the default limit for existing public webhooks.
- Sandboxing mutually malicious native UI plugin bundles.

## Implementation Plan

See [the implementation plan](../../../plans/plugin-voice-extraction-host/plan.md).
