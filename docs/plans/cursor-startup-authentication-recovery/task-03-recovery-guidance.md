---
id: "03-recovery-guidance"
title: "Show actionable authentication guidance"
status: done
wave: 3
depends_on:
  - "02-durable-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-AUTHENTICATION-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-SESSION-AUTHENTICATION-001.2
  - AC-AGENTS-SESSION-AUTHENTICATION-001.3
  - AC-AGENTS-SESSION-AUTHENTICATION-001.4
  - AC-AGENTS-SESSION-AUTHENTICATION-001.5
  - AC-AGENTS-SESSION-AUTHENTICATION-001.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.34
system_design:
  - ../../specs/agents/system-design/session-authentication-recovery.md
---

# Task 03: Show actionable authentication guidance

## Summary

Show provider authentication guidance in the existing active recovery card, including while method metadata is unavailable.
Prove desktop and phone recovery through a disposable ACP failure and real backend persistence.

## In scope

- Add a shared typed authentication presentation helper, provider-aware localized summary, and durable-cause fallback.
- Reuse `SessionRecoveryCard`, `ActionMessageDetails`, and existing auth panels without duplicate active controls.
- Show supported methods when usable and generic guidance for empty, unavailable, or non-executable method lists.
- Keep a protocol method ID separate from any terminal command; keep structured terminal commands copy-only unless validated executor-bound terminal context is available.
- Apply authentication labels and validated retry eligibility in the common recovery-choice pipeline.
- Preserve branch, history-continuation, eligible read-only workspace, inspection-busy, and guard behavior for authentication failures.
- Apply shared sizing to authentication controls reused in recovery.
- Add a private ACP fixture and desktop/phone browser coverage through the actual startup pipeline.
- Cover navigation, reload, explicit post-login retry, pending/failure/readiness, safe details, and retained history.
- Add the public authentication recovery section when implementation is complete.

## Out of scope

A new authentication continuation API, automatic credential changes, static Cursor shell commands from error prose, new recovery surfaces, and native-account repair claims.

## Acceptance

1. Component tests show methods and fallback guidance from persisted typed evidence, including no loaded transcript metadata and a released execution.
   A current-session native identity preserves Resume without matching transcript metadata; confirmed absence permits Restart, and unknown eligibility does not offer a context reset.
   The card identifies the configured provider and offers only eligible existing retry operations.
   Authentication failures retain the shared pipeline's inspection prerequisite, branch and history actions, eligible read-only workspace recovery, and guard filtering.
   New copy is localized in every shipped locale and updates when the locale changes.
2. Desktop browser tests prove a real ACP startup rejection becomes consistent durable/UI authentication evidence and survives navigation and reload.
   After the fixture's explicit login step, retry uses the prepared workspace and only authoritative readiness restores messaging.
   A failed retry retains its own cause and prior history. No native conversation is fabricated.
3. Phone tests perform the same recovery by touch, with measured 44-pixel targets, no horizontal overflow, wrapped safe details, and correct focus/pending behavior.
   Structured terminal commands are copyable with execution-host instructions; they do not launch in the Kandev host shell without validated executor context.
   Visible and copied details exclude privacy sentinels.
   Public guidance identifies the execution host and distinguishes provider authentication from repository/MCP setup.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview) as the structural contract.
This excerpt applies to authentication criteria .2-.4 and .6 and recovery criteria 006.17 and .34.

```text
Desktop: Cursor authentication is required
         Sign in on the execution host, then restart.
         [Login guidance]
         [Restart session] [Eligible workspace recovery]
         > Technical details

Phone:   Cursor authentication is required
         [Login guidance                    ]
         [Restart session                   ]
         [Eligible workspace recovery       ]
         > Technical details
```

Use generic guidance when methods are unavailable or cannot execute after teardown.
Keep the card at the composer location and retain one compact transcript history entry.
The existing bounded recovery region owns expanded content scrolling.
Phone actions stack; desktop actions wrap. Do not add a nested details scroller or another mobile state owner.
Pending uses the existing live status and disables equivalent controls.

## Verification

Run from the repository root. In a fresh worktree, install dependencies once before the first package command.
Write failing component and browser assertions before changing presentation.
After implementation, run the complete block sequentially:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-authentication-recovery.test.ts components/task/chat/session-recovery-card-authentication.test.tsx components/task/chat/messages/auth-methods-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-authentication-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-authentication-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds current backend/web artifacts and tears down its owned instances.
Confirm each selected project discovers its intended spec. Do not overlap browser runs or increase their worker budget.
Use causal backend/WS barriers and disposable login-state files; do not mutate a developer account or live task.
Keep screenshots with the work-order results when they help compare UI-01 and UI-02.

## Files likely touched

- `apps/web/lib/session-authentication-recovery.ts` and `.test.ts` (new).
- `apps/web/components/task/chat/session-recovery-model.ts`, `session-recovery-card.tsx`, and `session-recovery-card-authentication.test.tsx` (new test).
- `apps/web/components/task/chat/messages/action-message-details.tsx`, `auth-methods-panel.tsx`, and `auth-methods-panel.test.tsx` (new test).
- `apps/web/lib/active-session-recovery.ts` only if normalized cause forwarding needs correction, with its existing focused tests included in validation.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/{task,chat}.json` as required by the new keys.
- `apps/web/e2e/helpers/session-authentication-recovery.ts` (new), using the existing startup-process fixture pattern.
- `apps/web/e2e/tests/session/session-authentication-recovery.spec.ts` and `mobile-session-authentication-recovery.spec.ts` (new).
- `docs/public/agents-and-profiles.md`.

## Dependencies

Work order 02 supplies durable classification and safely projected method metadata.
Use its repository-reopen test as restart evidence; a page reload alone does not prove backend restart persistence.

## Risks

A protocol method such as `cursor_login` needs an owned live client for direct authentication.
Do not create a working-looking control for an execution already released by recovery.
Synthetic login success does not prove a particular Cursor CLI version's account repair sequence.
Treat real Cursor verification as a separate recorded observation when a disposable native environment is available.

## Parallelism

`sequential`

## Inputs

- [Design: login guidance and desktop/phone composition](../../specs/agents/system-design/session-authentication-recovery.md#login-guidance-retry-and-native-identity).
- [Plan previews and E2E matrix](plan.md#ascii-ui-preview).
- Shipped `SessionRecoveryCard`, `RecoveryActions`, and `auth-methods-panel.tsx`.
- Existing `agent-startup-process-diagnostics` helper and desktop/phone specs.
- Mobile parity, E2E fixture/cleanup guidance, frontend scoped instructions, and `docs/i18n.md`.

## Results

- Added provider-aware authentication guidance, durable-cause fallback, localized copy, and shared recovery-card presentation.
- Kept explicit retry eligible for identity-free startup authentication when the projected session state is `WAITING_FOR_INPUT`; generic waiting, non-start failures, dismissed causes, and sessions with native identity stay ineligible.
- Used the current session's canonical ACP identity to preserve Resume when transcript metadata is absent. Unknown identity eligibility no longer defaults to the context-resetting Restart action.
- Applied authentication eligibility and labels inside the shared recovery-choice pipeline. Mounted-card regressions cover branch, history-continuation, read-only workspace restoration, inspection-busy handling, and unknown identity.
- Removed the structured terminal-auth launch action from recovery. Advertised commands remain copyable with execution-host instructions, including after execution release; no command is sent to the Kandev host shell.
- Preserved composer focus after successful recovery when the recovery card owned focus, including the pending-control focus transition on mobile.
- Focused review regressions passed across 9 web test files (141 tests). `pnpm run typecheck`, `pnpm run lint`, and `pnpm run i18n:check` passed.
- Focused backend regression passed:
  `(cd apps/backend && go test ./internal/orchestrator -run '^TestEligibleInitialSubmissionRecoveryAllowsWaitingAuthenticationBootstrap$' -count=1)`.
- Backend race suite passed across orchestrator, executor, and lifecycle:
  `(cd apps/backend && go test -trimpath -tags fts5 -race -p 1 ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'Authentication|BootstrapFailureProjection|BootstrapFailureSuccessorFence|AuthenticateBySessionID' -count=1)`.
- Changed-file Go lint passed for `initial_submission_recovery.go` and `recovery_authentication_test.go`.
- Desktop Chromium and mobile Chrome recovery E2E specs each passed against freshly built backend and web artifacts after the review corrections.
- Public documentation checks passed: 63 validator tests and 55 published pages. Catalog validation covered 450 decisions and 1997 specifications; full specification lint passed.
- `git diff --check` and trailing-whitespace checks across 60 changed files passed.
- Native Cursor login and account repair were not attempted. The synthetic login verifies recovery flow only and does not demonstrate native Cursor account repair. PR #5298 was created from this implementation.

### PR review follow-up

- Kept authentication recovery inside the shared choice pipeline; mounted-card tests cover branch continuation, history continuation, eligible read-only workspace restore, and inspection prerequisites.
- Preserved Resume eligibility from canonical current-session ACP identity when transcript metadata is missing. A missing identity or token now remains unknown unless an initial startup cause or explicit no-token evidence confirms Restart eligibility.
- Confirmed advertised terminal commands remain copy-only during released-execution recovery. The mounted card test verifies a terminal-auth method exposes no Run in terminal control.
- Added touch recovery focus ownership: tapping an explicit recovery action records the focus handoff, and cleanup finds the composer in the same session panel after the card is removed. Focused mounted-card coverage and the mobile recovery E2E pass.
- The E2E fixture rejects whitespace-bearing temporary command paths, and the desktop spec polls durable API messages before asserting transcript rendering.
- Current focused frontend run passed across four test files (26 tests). `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run build:vite` passed.
- Mobile MCP catalog E2E passed (4 tests). Desktop authentication recovery and mobile authentication recovery E2Es each passed (1 test).
- Backend verification passed for the full MCP server, ACP adapter, process manager, and API package suites; the full backend build and changed-file Go lint passed. The focused orchestrator regression also passed.
- Raw ACP error data remains redacted from authentication WebSocket responses and logs. Out-of-scope locale edits were removed.
- PR CI and thread-resolution status are recorded at the final fixup checkpoint. Native Cursor login and account repair were not attempted.
