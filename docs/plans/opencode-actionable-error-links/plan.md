---
spec: docs/specs/agent-stall-recovery/spec.md
decision: docs/decisions/2026-08-07-allowlisted-provider-action-links.md
created: 2026-08-07
status: implemented
---

# Implementation Plan: OpenCode Actionable Error Links

## Outcome

Carry an optional, validated OpenCode remediation URL from an ACP/managed
diagnostic into Kandev's existing recoverable-error lifecycle, then render it
as a localized external link in every recovery surface. The short error remains
the primary copy; raw stderr and raw ACP error payloads remain excluded.

This plan is an incremental repair to the completed OpenCode terminal-error
surfacing work. It does not change prompt correlation, retry behavior, session
states, or recovery actions.

## Mobile design contract

The recovery link is an inline addition to the existing recovery cards and
notices; there is no new navigation or surface.

- Entry point: same chat/office surface on every viewport.
- Nearest shipped exemplar: the provider-quota recovery card
  (`mobile-provider-quota-recovery.spec.ts`); the link reuses its inline
  composition and 44px phone touch sizing (`min-h-11` below `sm`, `min-h-8`
  above).
- Presentation: inline text link next to the existing recovery actions —
  no drawer, dialog, or new route (content depth does not justify one).
- Scroll owner: unchanged chat panel; the link is `break-words`-safe and adds
  no horizontal scroll.
- Touch/focus: minimum 44px phone target, native anchor keyboard reachability,
  `target="_blank"` + `rel="noopener noreferrer"`.
- Mobile Playwright: `mobile-provider-remediation-link.spec.ts` proves the
  link, focus, 44px target, URL-free details, and zero document overflow.

## Root-cause constraint

The reported OpenCode 1.18.5 ACP failure contains only the short message in
Kandev's ACP error, logs, and managed stderr. OpenCode's TUI has additional
provider-side context, including the workspace URL, but that context is not
transported by the ACP service-failure response. No Kandev-only parser can
display bytes that never crossed the managed process boundary.

The Kandev half therefore supports the optional field now and keeps a truthful
fallback. A separate upstream OpenCode change is required for the exact
1.18.5-style failure to produce a link: OpenCode must emit an optional
`action_url` in its ACP service-failure data or in its already-supported
structured error-only stderr record. The plan must not add a private-log tail or
raw-error side channel to work around that dependency.

## Contract and security rules

- Add optional `remediation_url` to the normalized safe provider diagnostic and
  persist the same field in `LastAgentError` and recovery metadata.
- Accept only `https://opencode.ai/workspace/<safe-workspace-id>/go` with an
  exact host, HTTPS scheme, no userinfo/query/fragment, bounded identifier, and
  bounded total length.
- Extract the URL before sanitizing the provider message; the message must
  remain URL-free and identifier-redacted.
- For a future ACP `RequestError`, read only the structured `action_url` and
  safe message fields; never derive a browser link by copying `err.Error()`.
- Keep the URL out of generic logs, recent-stderr projections, raw ACP payloads,
  and technical text. Render it as a separate link with `noopener noreferrer`.
- If validation or transport fails, preserve the current generic recovery card
  and current short message.

## Backend work

### 1. Normalize transport-provided action URLs

Extend the existing OpenCode stderr parser and ACP prompt-error projection with
one shared allowlist validator. Structured stderr extracts a valid URL from the
provider error before sanitization; future ACP service-failure data accepts only
the explicit structured field. The provider diagnostic remains bounded and
optional, and all existing session/generation correlation rules stay intact.

### 2. Persist and publish the safe field

Extend `LastAgentError` with the validated URL and add the URL to generic
recovery-message metadata independently of quota classification. Reuse the
existing session metadata and `session.state_changed` snapshot so reloads,
Kanban banners, and Office failed-session rows see the same value. Do not change
the session's plain `error_message` contract.

### 3. Preserve existing classification behavior

Keep quota classification and `error_output` URL-free. A provider diagnostic may
carry a remediation URL even when it is not quota-classified, so the generic
recoverable card can still show the link. Tests must prove that URL-bearing
metadata never appears for invalid or untrusted inputs.

## Frontend work

### 4. Render one shared, mobile-safe link affordance

Add defense-in-depth URL validation at the browser boundary and a small shared
localized link component. Use it from:

- Kanban `ActionMessageDetails` and the persistent last-agent-error notice;
- Office `RunErrorEntry`, including the live `session.state_changed` metadata
  projection.

The link is separate from the collapsed technical text, opens in a new tab with
`rel="noopener noreferrer"`, and has a minimum 44px touch target on phones.
The existing recovery buttons, error copy, and chat scroll ownership remain
unchanged. New labels go through the existing `task`/`chat` translation files
and their pseudo-locale counterparts.

## Tests and verification

### Backend focused tests

- Parser accepts the observed OpenCode workspace route, stores it only in
  `remediation_url`, and keeps the URL/workspace ID out of `message` and
  sanitized stderr.
- Parser rejects wrong hosts, schemes, paths, query/fragment components,
  malformed IDs, overlong values, and unrelated URLs.
- ACP structured error data with `action_url` follows the same allowlist; absent
  or malformed data keeps the current generic error path.
- Lifecycle and orchestrator tests prove the field survives only through the
  winning error event, is persisted in `LastAgentError`/recovery metadata, and
  is absent for invalid input.

### Frontend unit tests

- `ActionMessage` renders a validated link for generic and quota recovery while
  preserving collapsed technical details and existing actions.
- `readLastAgentError` and the persistent notice retain/render the safe field.
- Office session mapping and `buildRunErrorsFromSessions` preserve the field;
  `RunErrorEntry` renders the same link and remains safe for invalid metadata.
- English and pseudo-locale checks pass; no hardcoded user-facing copy is added.

### E2E tests

Add seeded desktop and `mobile-chrome` scenarios that inject only the safe
`remediation_url` metadata. Verify the link destination, no URL in the
sanitized message, keyboard/touch reachability, 44px controls, and no
horizontal overflow. Add a no-link case for the current short ACP failure.
These tests must not invoke a real provider, exhaust quota, or read an
OpenCode log.

Targeted commands:

```text
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle ./internal/agent/runtime/routingerr ./internal/orchestrator -run 'Test(OpenCode|ProviderError|RecoverableFailure|RecoveryStatus)' -count=1
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx lib/session-last-agent-error.test.ts components/task/simple/chat-entries.test.ts lib/remediation-url.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run --no-build tests/session/provider-remediation-link.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-provider-remediation-link.spec.ts
cd apps/web && pnpm e2e:run --no-build tests/office/provider-remediation-link.spec.ts
```

## Likely files

Backend:

- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/orchestrator/recovery_actions_test.go`

Frontend:

- `apps/web/lib/session-last-agent-error.ts`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/app/office/tasks/[id]/page.tsx`
- `apps/web/app/office/tasks/[id]/types.ts`
- `apps/web/components/task/simple/chat-entries.ts`
- `apps/web/components/task/simple/components/run-error-entry.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/en/chat.json`
- matching pseudo-locale files and focused tests

## Sequencing and handoff

The work is sequential because the frontend consumes the persisted metadata
contract. There is no safe parallel implementation wave:

1. Normalize and test the transport contract.
2. Persist/publish it through the existing lifecycle.
3. Render both recovery surfaces and live Office updates.
4. Run focused unit, backend, i18n, desktop, and mobile verification.

The external OpenCode emission change is a prerequisite for live URL coverage
of the reported 1.18.5 error, but not for Kandev's parser, persistence, or
seeded UI tests. If upstream does not emit the field, Kandev must retain the
short-error fallback and report that limitation rather than exposing private
logs.

## Verification Results

Implemented 2026-08-07.

- Backend allowlist validator `NormalizeOpenCodeActionURL` in
  `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_remediation.go`,
  shared by the stderr parser (inline URL or `action_url` field) and the
  structured ACP `RequestError` projection. `ProviderError.RemediationURL`,
  `LastAgentError.RemediationURL`, recovery-message `remediation_url`, and the
  `task_session.error_changed` event all carry the field; `session.state_changed`
  carries it via the persisted `session_metadata`.
- Frontend: `lib/remediation-url.ts` browser-edge validation, shared
  `components/task/remediation-link.tsx` (`target="_blank"`, `rel="noopener noreferrer"`,
  min-h-11 touch target), wired into `ActionMessageDetails` (generic + quota
  cards), the persistent `LastAgentErrorNotice`, and Office `RunErrorEntry`
  (initial API data + live `session.state_changed` metadata merge).
- Generic logs: the `session.state_changed` metadata debug echo redacts
  `remediation_url` (`redactRemediationURL` in `task_notifications.go`).
- Verification runs (all green):
  - `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle ./internal/agent/runtime/routingerr ./internal/orchestrator -run 'Test(OpenCode|ProviderError|RecoverableFailure|RecoveryStatus)' -count=1` — passed.
  - Full touched-package backend suites (acp, streams, task/models, orchestrator,
    lifecycle, agentctl api/process, gateway/websocket) — passed.
  - `golangci-lint run ./... --new-from-rev=<base> --timeout=5m` — no issues.
  - `pnpm --filter @kandev/web test -- --run lib/remediation-url.test.ts lib/session-last-agent-error.test.ts components/task/chat/messages/action-message.test.tsx components/task/chat/message-list-shared.test.tsx components/task/simple/chat-entries.test.ts` — 106 passed.
  - Full web unit suite — 1236 files / 9669 tests passed.
  - `pnpm run typecheck`, `pnpm run i18n:check`, `pnpm run i18n:ratchet` — all clean (pseudo locale regenerated).
  - Desktop E2E `tests/session/provider-remediation-link.spec.ts` — 3 passed (link render + keyboard focus, short-error fallback, last-agent-error notice).
  - Mobile E2E `tests/session/mobile-provider-remediation-link.spec.ts` (mobile-chrome) — 1 passed (44px target, focus, overflow).
  - Office E2E `tests/office/provider-remediation-link.spec.ts` — 2 passed (link on RunErrorEntry, no-link for invalid/absent metadata).

External dependency stands: OpenCode must emit `action_url` (ACP structured
error data or structured stderr) for the live 1.18.5-style failure to show the
TUI workspace URL; Kandev keeps the short-error fallback until then.
