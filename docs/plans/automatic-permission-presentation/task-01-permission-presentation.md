---
id: "01-permission-presentation"
title: "Carry automatic presentation through permission delivery"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.11
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.12
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.13
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.14
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.15
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.16
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.17
system_design:
  - ../../specs/agents/system-design/automatic-permission-presentation.md
---

# Task 01: Carry Automatic Presentation Through Permission Delivery

## Summary

Deliver the complete message-to-chat correction with backend and component
regressions. Keep automatic intent in the initial durable message and preserve
the existing provider response authority.

## In scope

- Add `PermissionRequestMessageOptions` and typed presentation metadata.
- Pass candidate intent through `MessageCreator` and the idempotent create.
- Add an identity-qualified conditional presentation update through the
  repository, task service, and message-creator adapter. Preserve audit and status.
- Promote only confirmed live, unclaimed failures to interactive presentation.
  Keep dispatched, terminal, uncertain, and competing claims safe.
- Apply one classifier to standalone requests, tool-attached permission state,
  Kandev tool rows, and attention indicators.
- Keep SQL and browser current-turn/newest-row selection in the same order.
- Cover replay, hydration, stale updates, mixed sessions, and legacy events.

## Out of scope

- New provider policy, profile settings, runtime flags, or schema migrations.
- Filtering the live runtime list needed by automatic resolution.
- New recovery actions, audit viewers, or unrelated permission history changes.

## Acceptance

1. The first published/hydrated candidate already carries automatic intent.
   Rendering it produces normal tool progress without approval UI or input attention.
2. A verified live failure before claim becomes interactive. A dispatch claim
   or terminal decision never becomes a second answer opportunity.
3. Existing audit ordering, replay barriers, manual decisions, and newest-row
   authority remain valid across both SQL dialects and chat renderers.

## UI preview

These excerpts match [the combined preview](plan.md#ui-preview).

| View | Required result |
| --- | --- |
| UI-01: Automatic approval | Running tool row, no approval fragment or amber input state |
| UI-02: Interactive fallback | Existing title and provider choices, reachable by keyboard and touch |
| UI-03: Unavailable decision | Existing unavailable/recovery feedback, no answer controls |

Desktop and phone share this composition. Keep the transcript scroll owner,
composer geometry, focus, and existing phone touch targets.

## Verification

Run the workspace install once if this worktree lacks its dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run the behavioral RED tests before production edits. The proposed new names are:

- `TestAutomaticPermissionPresentationIsStoredBeforePublication` in
  `internal/backendapp/adapters_permission_presentation_test.go`.
- `automatic candidates never render an input prompt` in
  `lib/permissions/presentation.test.ts` and
  `components/task/chat/messages/permission-presentation.test.tsx`.

After implementation, run from the repository root:

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/orchestrator ./internal/task/service ./internal/task/repository/sqlite -run 'Test.*(AutomaticPermission|PermissionPresentation|PendingAction|PendingInteraction|HandlePermissionRequest|PermissionDecision)' -count=1)
(cd apps/backend && go test ./internal/integration -run '^Test(AutoApprovedPermissionDecisionPersistsInSessionReplay|AutomaticPermissionPresentationSurvivesReplay)$' -count=1)
(cd apps/web && pnpm exec vitest run lib/permissions/presentation.test.ts hooks/processed-message-filtering.test.ts lib/utils/pending-clarification.test.ts components/task/chat/messages/use-permission-handlers.test.ts components/task/chat/messages/permission-presentation.test.tsx lib/state/slices/session/session-slice.merge-messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/permissions/presentation.ts lib/types/permission.ts lib/utils/pending-clarification.ts hooks/processed-message-filtering.ts components/task/chat/messages/use-permission-handlers.ts components/task/chat/messages/permission-request-message.tsx)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Run `scripts/lint-go-changed` on the actual changed Go files, including new files:

```bash
python3 - <<'PY'
from pathlib import Path
import subprocess
tracked = subprocess.check_output(['git', 'diff', '--name-only', '-z', '--diff-filter=ACMR', '--', 'apps/backend/'])
added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z', '--', 'apps/backend/'])
files = sorted({p.decode() for p in (tracked + added).split(b'\0') if p.endswith(b'.go')})
files = [p for p in files if Path(p).is_file()]
if files:
    subprocess.run(['bash', 'scripts/lint-go-changed', *files], check=True)
PY
```

Required cases include pre-claim failure, dispatch/finalization failure, replaced
request identity, competing claim, idempotent replay, legacy unmarked prompts,
and a newer automatic row after an older pending manual row. Also cover two
sessions where only one has a current manual request. A provider receives at
most one response for each exact request.

## Files likely touched

- `apps/backend/internal/backendapp/adapters.go` and new
  `adapters_permission_presentation_test.go`.
- `apps/backend/internal/orchestrator/service.go`, `event_handlers_git.go`,
  `agent_permissions.go`, `event_handlers_git_permission_test.go`, and their
  affected `MessageCreator` test doubles.
- `apps/backend/internal/task/models/models.go` and `repository/interface.go`.
- `apps/backend/internal/task/repository/sqlite/message.go`,
  `pending_interactions.go`, and new `message_permission_presentation.go`.
- Matching repository tests, including `message_pending_postgres_test.go`
  and `pending_interactions_test.go`.
- `apps/backend/internal/task/service/service_messages.go` and
  `service_messages_permission_test.go`.
- `apps/backend/internal/integration/agent_permission_mcp_test.go`.
- New `apps/web/lib/permissions/presentation.ts` and its test.
- `apps/web/lib/types/permission.ts` and `lib/utils/pending-clarification.ts`.
- `apps/web/hooks/processed-message-filtering.ts` and its tests.
- `apps/web/components/task/chat/messages/use-permission-handlers.ts`,
  `permission-request-message.tsx`, and new `permission-presentation.test.tsx`.
- Existing ordinary/Kandev tool renderers if their shared parser is insufficient.
- `apps/web/lib/state/slices/session/session-slice.merge-messages.test.ts`.

New Go helpers and tests remain inside these owned packages. Keep new copy in
the locale catalogs if existing localized feedback cannot express the state.
Use all supported languages and the existing Traditional Chinese generator.

## Dependencies

None. Read the plan, requirement, scoped backend/web guidance, and design first.

## Risks

The marker is presentation data, not a grant. Do not set approved status early.
A failure after dispatch requires non-actionable feedback. Do not erase its
claim or infer nondelivery from an error alone.

## Parallelism

`sequential`

## Inputs

- [Requirement 003](../../specs/agents/requirements/permission-control-integrity.md#req-agents-permission-control-integrity-003-auto-approve-approves-or-prompts-and-never-denies).
- [Presentation design](../../specs/agents/system-design/automatic-permission-presentation.md).
- [Live authority ADR](../../decisions/2026-08-11-live-agent-permission-authority.md).
- `TestAutomaticPermissionCandidatePersistsBeforeProviderApproval` and
  `TestListPendingInteractionsAgreesWithPendingActionProjection`.

## Results

Completed on 2026-10-30, with static-review remediation.

- Added typed `PermissionPresentation` values and
  `PermissionRequestMessageOptions`. New permission rows persist `automatic` or
  `interactive` in the same idempotent create, before publication.
- Added an identity-qualified conditional presentation update through the
  repository, task service, and message-creator adapter. Promotion requires the
  exact row, its original `automatic` presentation, and no resolution claim.
- Failed automatic decisions promote to `interactive` only after revalidating
  the live request generation. Dispatch and finalization failures retain their
  claim and record non-actionable `unavailable` presentation. A failed live
  enumeration, a withdrawn or replaced request, or an automatic claim whose
  read-back failed also settles to `unavailable` instead of remaining hidden.
  Settlement uses a bounded cleanup context that survives request cancellation.
  A competing dispatching resolver or accepted outcome keeps ownership.
- Added an automatic-only claim guard. A persisted candidate replayed after a
  pre-claim failure was promoted to `interactive` cannot be claimed or delivered
  automatically, even after storage recovers.
- Added the shared web classifier. Transcript filtering, permission rendering,
  tool-attached permission state, and attention selectors now agree. Automatic
  rows mount no controls; unavailable rows reuse localized feedback. Attached
  generic and registered Kandev tool rows now show a non-actionable feedback
  fragment because their standalone permission row is deduplicated.
- Pending-action and pending-interaction SQL projections rank the newest
  permission before classifying its presentation, so a newer automatic row
  cannot resurrect an older manual request. Live agent permission enumeration
  remains unchanged.
- Message merge freshness preserves a newer presentation or terminal status when
  a delayed duplicate or snapshot arrives.
- Post-delivery finalization failures keep their internal uncertain subtype while
  preserving the established external MCP `permission_audit_failed` code and
  message.

Verification passed:

- Backend targeted suites for `backendapp`, `orchestrator`, `task/service`,
  `task/repository/sqlite`, `integration`, and `mcp/handlers`, including claim
  read-back failure, enumeration failure, competing resolver, automatic-only
  replay, and finalization-error classification regressions.
- Per-package `golangci-lint --new-from-rev` runs: 0 issues.
- Web unit suites: 138 tests passed across the classifier, filtering, attention,
  handler, attached-feedback, component, merge, and browser-gate suites.
- `tsc --noEmit`, ESLint, and `i18n:check` passed.
- Desktop and phone browser cases passed; see task 02 for counts.
