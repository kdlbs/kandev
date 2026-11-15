---
id: "03-google-sign-in"
title: "Complete local Google sign-in"
status: in_progress
wave: 3
depends_on:
  - 02-subscription-policy
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-005
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-015.1
  - AC-AGENTS-ANTIGRAVITY-ACP-015.2
  - AC-AGENTS-ANTIGRAVITY-ACP-015.3
  - AC-AGENTS-ANTIGRAVITY-ACP-005.5
  - AC-AGENTS-ANTIGRAVITY-ACP-005.6
  - AC-AGENTS-ANTIGRAVITY-ACP-010.4
  - AC-AGENTS-ANTIGRAVITY-ACP-010.5
  - AC-AGENTS-ANTIGRAVITY-ACP-010.6
  - AC-AGENTS-ANTIGRAVITY-ACP-011.2
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 03: Complete local Google sign-in

## Summary

Let the operator complete the native ACP Google login from agent settings.
Use the vendor's local callback and refresh the real model catalog before reporting readiness.

## In scope

- Add one bounded in-memory sign-in attempt to hostutility using a dedicated temporary native agentctl instance.
- Add operator-only start/status/cancel endpoints and DTO/API contracts described in the design.
- Reuse authenticated configure/start/initialize/authenticate methods. Add transient `agent.auth.status` responses and trace redaction rather than a new ACP transport.
- Retain only validated active-attempt URLs in memory. Clear them on terminal states; reject foreign, stale, or late results.
- Keep ten-minute overall expiry, idempotent start/cancel, reload reattachment, backend shutdown cleanup, and responsive status during pending auth.
- Keep successful consent distinct from failed model discovery. Retry the probe without asking for consent again unnecessarily.
- Add a focused Antigravity settings dialog, task auth guidance link, and phone sheet/focused view.
- Preserve task drafts and return navigation. Explain separate ACP login and same-machine consent.
- Use localized copy in seven locales, accessible statuses/focus, and existing responsive/mobile exemplars.
- Add synthetic setup E2E fixtures without real browser OAuth or subscription calls.

## Out of scope

Hosted callbacks, callback paste/relay, account switching, token refresh code, arbitrary auth-method selection, and other agents' PTY login flows.

## Acceptance

1. Only an authorized operator can start/read/cancel a bounded attempt; native OAuth success plus a fresh probe establishes ready models.
2. Denial, expiry, repeated start, cancellation, reload, restart, and late responses preserve coherent state and leak no URL into durable output.
3. Desktop and phone use the reviewed flow, keep actions reachable, and explain that consent finishes on the Kandev machine.

## ASCII UI preview

Excerpt of [UI-02](plan.md#ui-02-personal-google-sign-in), covering 010.4-010.6 and 015.1-015.3.

```text
Desktop focused dialog                 Phone inset sheet
+--------------------------------+     +----------------------------+
| Sign in to Antigravity    Close |     | Sign in to Antigravity     |
| Use your Google subscription.  |     | Open the copied link on    |
| Finish on this Kandev machine. |     | the Kandev machine.        |
| Waiting for Google sign-in     |     | Waiting for Google sign-in |
| [Open Google sign-in] [Copy]    |     | [        Copy link       ] |
|                       [Cancel] |     | [         Cancel         ] |
+--------------------------------+     +----------------------------+
```

Before starting, Sign in with Google is primary. Starting/checking announces progress and keeps Cancel.
Success shows Ready; denial/expiry clears the link and offers Sign in again.
A catalog failure after consent says Signed in and offers Retry model discovery.
Phone uses 44px actions and the existing drawer patterns; it does not present a phone loopback callback as supported.
Closing cancels its active owned attempt and restores focus; reload reattaches while the attempt remains active.

## Verification

Run from the repository root after Task 01's pnpm bootstrap.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/hostutility ./internal/agent/capabilities/handlers ./internal/agentctl/server/api -run 'TestAntigravity' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/hostutility ./internal/agentctl/server/api -run 'TestAntigravity' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/antigravity-login-dialog.test.tsx lib/api/domains/antigravity-auth-api.test.ts)
(cd apps/web && pnpm exec eslint components/settings/antigravity-login-dialog.tsx components/settings/installed-agent-card.tsx components/task/auth-methods-indicator.tsx lib/api/domains/antigravity-auth-api.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/antigravity-setup.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-antigravity-setup.spec.ts)
git diff --check
```

Named Go tests: `TestAntigravityLoginAttempt`, `TestAntigravityLoginAuthorization`, and `TestAntigravityAuthStatusWhilePending`.
Cover arbitrary URL rejection, authority, transient-only URL storage, cleanup, stale IDs, and the ten-minute deadline with a fake clock.
Keep status requests serviceable while Authenticate is waiting. Verify tracing redaction at the agentctl boundary.
Browser tests exercise start/copy/cancel, denial/expiry, reload, authenticated-but-probe-failed, model refresh, focus, and phone wrapping/touch targets.
Run changed-Go lint for all changed/new Go files.

## Files likely touched

- `apps/backend/internal/agent/hostutility/antigravity_login.go`, `antigravity_login_test.go` (new), manager lifecycle wiring.
- `apps/backend/internal/agent/capabilities/handlers/handlers.go`, Antigravity handlers and tests.
- `apps/backend/internal/agentctl/server/api/agent.go`, transient auth-status action and tests.
- `apps/backend/internal/agentctl/server/adapter/` optional auth status contract and Antigravity state.
- `apps/backend/internal/agent/runtime/agentctl/agent.go` and transport tracing redaction.
- `apps/web/lib/api/domains/antigravity-auth-api.ts`, tests and API exports/types.
- `apps/web/components/settings/antigravity-login-dialog.tsx`, tests, installed card wiring.
- `apps/web/components/task/auth-methods-indicator.tsx` and existing task auth-required guidance.
- `apps/web/e2e/tests/settings/antigravity-setup.spec.ts`, `mobile-antigravity-setup.spec.ts`, shared helper, and backend mock support.
- `apps/web/src/locales/` for seven locales; generate the Traditional Chinese pair.

## Dependencies

Task 02 provides shared background authentication, safe URL parsing, and browser suppression.

## Risks

- Real consent needs the user. Synthetic CI success is not live native qualification.
- Remote/phone browsers cannot complete a callback to the host's localhost. Keep the limitation visible without building a relay.
- A broad generic stderr API or journaled event would persist consent state; use the transient restricted response.

## Parallelism

`sequential`

## Inputs

- Design sections Authentication, Setup UI, Persistence, and Security.
- Existing agentctl client methods, capabilities handlers, and installed-agent card.
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx`, responsive breakpoint helpers, and `useTouchDrawer`.

## Results

Implemented.

- Added a bounded in-memory Antigravity sign-in attempt to `hostutility.Manager`, using a dedicated temporary agentctl instance, existing configure/start/initialize/authenticate methods, a ten-minute deadline, cancellation, and cleanup.
- Added operator routes under `/api/v1/antigravity-acp/login` (start, latest/read, cancel) with `org.config.manage` and the interim settings interlock. The path is a sibling of `agent-capabilities` because Gin does not allow a static `antigravity-acp` child beside the existing `:type` wildcard.
- Added transient `agent.auth.status` request/response handling, the agentctl client method, consent-URL redaction in stderr sanitization, and attempt identity checks.
- Added the focused desktop dialog and phone drawer with start, status, open/copy, cancel, retry, and same-machine guidance. Wired the installed Antigravity card to the dialog.
- Added API and component tests.

Verification: backend targeted tests, `go test -race` for host utility/auth status paths, web Vitest, typecheck, and i18n checks. Real Google consent and live model discovery remain Task 07 evidence.

Review remediation: login attempts now fence terminal state until native cleanup completes, expire without retaining URLs, and reject a successor while cleanup is unfinished. Sign-in polling survives transient status failures, clears missing attempts after restart, fences stale responses, and closes only after confirmed cancellation. The phone sheet renders one action region.

Browser setup E2E specs remain open; earlier deterministic backend and component checks passed.
Those checks did not exercise the configure boundary described below.

### Sign-in launch regression, November 4

The Settings sign-in action sends `args[1:]` to a configure API that expects full argv.
Linux therefore sends only `--uid=` and fails before native startup.
The [focused repair package](../antigravity-sign-in-launch/plan.md) owns the correction and real HTTP-boundary regression.
That repair is implemented: the sign-in caller passes the complete argv and `TestAntigravityLoginConfigurePreservesCommand` reaches the controlled start marker through the real configure handler.
Keep this work order `in_progress`; the browser setup specs and full native qualification remain open.

### Sign-in stream startup regression, November 8

[Issue #5193](https://github.com/kdlbs/kandev/issues/5193) confirms a later caller
defect: `Client.StreamUpdates` returns nil after establishing its WebSocket, but
the login wait helper interprets that success as a closed stream and deletes
the instance before initialization/authentication. A temporary diagnostic
reproduced 20/20 false failures with the real client and a healthy local socket.
The [stream-startup repair](../antigravity-login-stream/plan.md) is implemented.
The caller opens the stream synchronously and its real-client WebSocket tests
cover successful initialization/authentication, handshake rejection, disconnect,
and cancellation. Focused regressions and the full hostutility race suite pass.
This work order's existing browser and native qualification gates remain open.
