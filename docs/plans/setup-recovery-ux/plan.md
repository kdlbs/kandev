---
created: 2026-10-02
status: in_progress
requirements:
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Setup recovery UX

## Overview

Show imported plugin authentication requirements as actionable warnings and keep
created Quick Chats inspectable through setup failures and navigation races.
Implement warning presentation first, then Quick Chat retention and recovery.
Implementation and local review corrections are complete, including desktop and
phone browser verification. Remote PR checks and review disposition remain pending.

## Evidence and diagnosis

Read-only inspection of the live instance on port 38429 and retained
`~/.kandev/logs/backend-logs.log` on 2026-10-02 established:

- Task `b389a65f-bcd2-4ed3-ad1a-adbad2365b15`, session
  `7c03969a-f1d9-4802-91e9-f91dd7daea64`, started its agent at
  15:36:04 +0100. The screenshot's failed plugin verification did not prevent
  the agent from starting. `verifyCursorProjectMCPImport` records every non-ready
  verification as failed; `prepare-progress.tsx` treats every failed step as an
  error, and `agent-mcp-prepare-actions.tsx` always colors its reason red.
- Quick Chats `1b64d874-21a7-475f-8281-ebdecd6e3a77` and
  `16b912d5-2a3c-41ed-abfb-6f1356a8ec23` received HTTP 200 creation responses at
  15:35:14.851 and 15:35:30.939. HTTP DELETE requests followed, with deletion
  events at 15:35:15.370 and 15:35:31.455. Runtime startup was then aborted by
  teardown. These were client-triggered deletions, not the backend's synchronous
  failure rollback. No "failed to start quick chat session" record was found.
- `useAgentSelection` deletes the returned task when its request generation was
  superseded. Tab switches, New Chat, and dialog dismissal reset that generation.
  Its existing test explicitly expects deletion after reset. This supplies a
  deterministic reproduction: delay creation response, let WS announce the tab,
  activate another tab, then release the response. The exact gesture during the
  reported incident remains unconfirmed; no browser event trace was captured.
- Independently, `httpStartQuickChat` deletes the ephemeral task on any synchronous
  launch failure, and the frontend catch only shows a toast. Both violate the
  requested inspectable failure UX once a conversation exists.

## Scope

Include typed authentication warning severity; retained created Quick Chats;
inline pre-session errors; existing guarded retry and deletion; desktop/mobile,
reload and reconnect coverage. Exclude new provider adapters, OAuth mechanics,
blanket downgrading of errors, retention-policy changes, and modal redesign.

## Technical approach

Follow the amendments in the two linked system designs. Keep failed readiness
records intact and derive presentation severity from exact kind/reason metadata.
Reuse warning-completion presentation. No schema migration is needed for warnings.

For Quick Chat, add retained identity to the endpoint's typed error envelope after
session allocation, persist the safe cause, and remove destructive stale-response
cleanup. Upsert late results without activation; preserve deletion tombstones.
Before session allocation keep setup values and an inline error. Reuse existing
launch recovery and prevent automatic retries while an active error is displayed.

| Provider / transport | Intended behavior | Evidence / unsupported fallback |
| --- | --- | --- |
| Cursor imported MCP, ACP or terminal | Auth-required verification is amber; tools remain unavailable | Typed reason fixture and live/hydrated UI tests; other codes retain errors |
| Other agents / unknown MCP metadata | Existing preparation semantics | Never infer auth from text or missing tools |
| Quick Chat ACP and passthrough | Retain accepted session on failure | Handler/store/component and desktop/mobile E2E; absent session keeps setup form |

## ASCII UI preview

### UI-01: Preparation authentication warning

Entry: task or Quick Chat conversation, completed environment preparation.
Current: red "Environment setup finished with errors", red verification cross.
Proposed shared desktop/phone content:

```text
[v] Environment setup finished with warnings
    [!] Verify connection: plugin-atlassian-atlassian
        Authentication is required.
        [Authenticate] [Retry connection]
```

The triangle and warning message are amber; a real fatal failure stays red.
On phones actions wrap into reachable 44px targets in the existing scroll region.
Maps to AC-AGENTS-MCP-PREP-002.6/002.7 and AC-AGENTS-MCP-PREP-003.5.

### UI-02: Retained Quick Chat error

Entry: Start chat, then a genuine setup failure. Current reported behavior loses
the created chat; the pre-session request catch exposes only a toast.

Desktop, persisted session:

```text
+--------------------------------------------------+
| [Cursor chat !] [Other chat] [+]               [x] |
| Environment setup failed                         |
| Safe failure details                             |
| [Retry]                                          |
|                                                  |
| Conversation history / preparation details        |
+--------------------------------------------------+
```

Phone, existing full-height Quick Chat surface:

```text
+----------------------------+
| Quick Chat           [Close]|
| [Cursor chat !] [Other chat]|
|----------------------------|
| Environment setup failed   |
| Safe failure details       |
| [Retry]                    |
|                            |
| Scrollable conversation    |
| and preparation details    |
|----------------------------|
| Existing composer / state  |
+----------------------------+
```

Pre-session form failure, both surfaces:

```text
Agent profile: [Cursor         v]
Repositories:  [retained choices]
[!] Could not start chat. Safe reason.
                        [Cancel] [Retry]
```

Structural requirements: retain the selected surface on failure, error text and
valid recovery controls in the existing content region, explicit tab deletion
through the existing confirmation, and no new nested overlay. Existing full-height
phone shell owns safe-area and viewport constraints; content scrolls internally.
Labels and spacing are illustrative and localized. Retry is shown only when the
existing error contract permits it. Maps to all ACs of
REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.

## Tests and E2E tests

Task 01 maps AC-AGENTS-MCP-PREP-002.6/002.7 and 003.5 to
`prepare-progress-status.test.ts`, `prepare-progress.test.tsx`, and
`agent-mcp-prepare-actions.test.tsx`. Add warning-only, mixed-error, fatal-overall,
legacy-hydration, and retry-clear cases. Render both viewport modes in focused
`chat/setup-recovery.spec.ts` and `chat/mobile-setup-recovery.spec.ts` scenarios.

Task 02 maps all five Quick Chat retention ACs to a new
`quick_chat_failure_retention_test.go`, `use-quick-chat-modal.test.ts`,
`quick-chat-setup.test.tsx`, `quick-chat-session-view.test.tsx`, and existing
quick-chat WS/store/reconnect suites. Reverse the existing reset-deletes-result
expectation and cover WS-before-HTTP, late failures, explicit deletion tombstones,
and mixed healthy/failed tabs. The same E2E files prove visible errors, no unwanted
DELETE, explicit recovery, reload, reconnect, and phone control reachability.
Use isolated mock fixtures, not the live instance or personal plugin credentials.

## Work orders

- [x] [Task 01: Authentication warning presentation](task-01-auth-warning.md)
- [x] [Task 02: Retained Quick Chat setup](task-02-retain-quick-chat.md)

## Documentation

Requirements/design amendments live with their existing owners. This routine
recovery correction needs no additional ADR; the designs preserve its rationale.
Updated public tasks/workflows recovery guidance in `docs/public/tasks-and-workflows.md`.
Existing delivery packages are historical evidence; these amendments do not change
their recorded implementation results.

## Prior implementation verification

The following results were recorded by the implementing agent before this review.
They do not validate the revised browser scenarios.

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node scripts/validate-public-docs.mjs`: passed, 47 published docs pages.
- `git diff --check`: passed.
- Backend tests: `go test ./internal/task/handlers -run 'TestQuickChatFailureRetention|TestHTTPListQuickChatSessions' -count=1`: passed.
- Frontend unit tests: `pnpm exec vitest run lib/prepare/agent-mcp-warning.test.ts components/session/prepare-progress-status.test.ts components/session/prepare-progress.test.tsx components/task/agent-mcp-prepare-actions.test.tsx components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-chat-setup.test.tsx components/quick-chat/quick-chat-session-view.test.tsx lib/state/slices/ui/quick-chat-sync.test.ts hooks/use-quick-chat-resync.test.ts lib/ws/handlers/tasks-quick-chat.test.ts`: passed (113 tests).
- E2E tests: desktop `e2e/tests/chat/setup-recovery.spec.ts` (4 tests) and mobile `e2e/tests/chat/mobile-setup-recovery.spec.ts` (2 tests): passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`: passed.

## Risks and remaining evidence limits

- The incident's exact browser gesture is unknown. The confirmed client DELETE
  and deterministic supersession path must not be presented as proof of that gesture.
- Stale-response retention must not steal focus or resurrect an explicitly deleted tab.
- Error envelopes must retain authorization, sanitized causes and existing status codes.
- Pre-session failure cannot restore a session that was never created. Keep the
  form and inline error locally; persisted-session failures survive reload.
- Existing automatic Quick Chat resumption must not hide a retained error.

## Local review and corrections (2026-10-02)

Review scope: working-tree snapshot over HEAD
`157502307f4292eb6de005b3c6317d22a45fbec2`, including untracked source and tests.
At that review checkpoint, no commit or remote publication was requested or performed.

Confirmed and corrected with failing regression tests:

1. Late successful and failed responses used `addQuickChatSession`, which selects
   the incoming tab. Use the non-activating, tombstone-aware event upsert instead.
   Test the hook with real Zustand actions, not only mocked call assertions.
2. `openQuickChat` and `addQuickChatSession` could recreate explicitly deleted
   conversations. Their shared upsert now rejects unexpired deletion tombstones.
3. Retention lookup used the canceled request context and interpreted lookup errors
   as absence. Resolve with a bounded cancellation-independent context that keeps
   caller identity; delete only after a successful empty session-list read. The
   mock now reproduces the real repository's no-primary-session error.
4. The automatic-recovery guard skipped only initial startup. Window focus and an
   in-flight status response could still resume. Gate focus, track guard changes
   in request generation, and invalidate pending work when the guard changes.
5. Quick Chat treated dismissed error history as active and could start recovery
   before session hydration. Use the shared active-error reader and wait for the
   persisted session row before automatic recovery.

Additional improvements: consolidate retained/successful response reconciliation,
remove unused destructive cleanup and stale comments, extract preparation step
icons and inline setup feedback to meet code-quality limits, and test the actual
retained-error parser. Strengthen desktop E2E to use real persisted identities,
WS-before-HTTP ordering, visible error/recovery controls, and reload instead of
invented IDs. The failure E2E seeds a durable failure projection; it is not proof
of a real provider bootstrap failure.

Validation at the initial review checkpoint:

- Targeted frontend run: 26 files, 258 tests passed, including the complete
  `components/quick-chat` folder, preparation/action tests, real-store retention,
  API parser, reconnect/WS, and session resumption tests.
- `GOCACHE=/tmp/kandev-setup-review-go-cache go test ./internal/task/handlers -run
  'TestQuickChatFailureRetention|TestHTTPListQuickChatSessions' -count=1`: passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, and targeted ESLint: passed.
- The managed desktop E2E run built successfully with a writable Go cache but
  all four scenarios stopped before test execution because macOS rejected
  Chromium's Mach rendezvous registration (`bootstrap_check_in`, permission
  denied 1100). These are environment launch failures, not passing E2E results.
  Phone execution has the same unavailable browser prerequisite and was not repeated.
- Revised rendered scenarios remain unverified in this environment. Run the task
  E2E commands in CI or a host that permits Chromium before marking delivery done.

Exact frontend regression command (from `apps/web`):

```bash
pnpm exec vitest run components/quick-chat lib/api/domains/workspace-api-retention.test.ts lib/prepare/agent-mcp-warning.test.ts components/session/prepare-progress-status.test.ts components/session/prepare-progress.test.tsx components/task/agent-mcp-prepare-actions.test.tsx lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts hooks/use-quick-chat-resync.test.ts lib/ws/handlers/tasks-quick-chat.test.ts hooks/domains/session/use-session-resumption.test.ts
```

## Publication validation follow-up (2026-10-02)

After filesystem permissions were restored, Chromium launched successfully. The
initial browser failures above are historical and superseded by these results.
Corrected E2E fixture expectations: identify persisted tabs by session identity,
provide the terminal-state completion timestamp, wait for the retained HTTP error,
and scope error/action assertions to the recovery message rather than duplicated
failure-banner content. Production behavior required no further changes.

- `pnpm e2e:run --host --no-build --project chromium tests/chat/setup-recovery.spec.ts tests/session/agent-mcp-preparation.spec.ts`:
  all five regression scenarios passed in the desktop run.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-setup-recovery.spec.ts tests/session/mobile-agent-mcp-preparation.spec.ts`:
  all three regression scenarios passed in the phone run.
- Each run also included disposable capture specs with `CAPTURE_PR_ASSETS=1`:
  three desktop and two phone captures passed, for 13 passing scenarios total.
  Captures show typed authentication warnings, retained failure/recovery details,
  and inline setup errors. Temporary specs were removed after capture.
- The host backend was freshly built with `GOCACHE=/tmp/kandev-setup-review-go-cache`
  and `GOFLAGS=-buildvcs=false` before the successful browser runs.
- The exact retained-error browser scenario seeds a durable failure projection;
  backend handler tests independently cover synchronous launch failure retention.

Local implementation verification is complete. Publication and exact-current-head
CI/review verification remain external delivery steps.

Publication hook follow-up: removed a redundant Go return and split the hook
retention tests and preparation-warning group to satisfy existing size limits.
The affected frontend rerun passed all 41 tests across three files. The broader
258-test result above precedes the file split; no regression assertions were removed.
