---
id: "03-recover-expired-continuations"
title: "Recover expired plugin continuations"
status: done
wave: 3
depends_on:
  - "02-restore-core-snapshots"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
system_design:
  - "../../specs/plugins/system-design/conversation-recovery.md"
---

# Task 03: Recover expired plugin continuations


## Replacement scope, 2026-09-16

The [conversation storage replacement](../conversation-storage-replacement/plan.md) owns the next implementation.
This file preserves historical scope and results. Do not execute its durable replay mechanics as new work.
The replacement work orders preserve public behavior and provide new source-reconciliation, upgrade, and E2E evidence.


## Summary

Let an idle plugin panel resume paging and live updates without remount.
Keep expired grants invalid and preserve the existing public error contract.

## In scope

- On-demand expiry detection and one shared rebind cycle per scope.
- Atomic cursor/snapshot replacement and stale continuation fencing.
- A bounded continuation retry after fresh hydration.
- Idle resume-token rejection through the same recovery mechanism.
- Expiry, removal, access, and concurrent-query regression cases.

## Out of scope

Token lifetime changes, broad HTTP-400 retry, new SDK methods, automatic polling,
new UI controls, and core transport redesign.

## Acceptance

- After more than ten idle minutes, loadMore recovers and returns newly projected IDs without remount. A later live event still applies.
- Concurrent callers share recovery. Distinct panels and queries retain isolation.
  Old responses cannot overwrite new state. Exhausted and terminal handles do no further reads.
- Invalid queries and revoked access remain non-retryable. Transient recovery errors preserve rows and permit a fresh explicit retry.

## TDD sequence

1. Add a scope/hook regression that advances beyond the issued token expiry.
   The mock must reject expired tuples rather than returning success for every request.
2. Add real token-manager rejection in `TestConversationContinuationRenewRejectsExpiredTokens`.
   Use the existing injectable clock. Keep pre-expiry renewal coverage.
3. Test concurrent callers, stale in-flight page/renewal, two queries, removal,
   disable/unmount, access revocation, and one live event after idle expiry.
4. Implement one generation-bound recovery through existing rebind listeners.
   Preserve the original load-more intent and count semantics from the design.
5. Add the browser expiry case to `conversation-recovery.spec.ts`.
   Use causal evidence and a guarded test clock if server expiry is required.
   Run the existing desktop/mobile plugin and core panel parity checks.

## Files likely touched

- `apps/web/lib/plugins/conversation-scope.tsx`
- `apps/web/lib/plugins/conversation-host.tsx`
- `apps/web/lib/plugins/conversation-host.test.tsx`
- `apps/web/lib/plugins/conversation-host-isolation.test.tsx`
- `apps/backend/internal/plugins/conversation_handlers_test.go`
- `apps/backend/internal/plugins/conversation_tokens.go` only for a test clock seam if necessary
- `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts` from Task 02
- Existing guarded E2E helpers only if needed for token timing
- `docs/plans/plugins/PLUGIN-API.md` for Host-only expiry recovery clarification

## Verification

From the repository root, after implementation/validation authorization:

```bash
(cd apps/backend && go test -tags fts5 ./internal/plugins ./internal/gateway/websocket)
(cd apps/web && pnpm exec vitest run lib/plugins/conversation-host.test.tsx lib/plugins/conversation-host-isolation.test.tsx)
(cd apps/web && pnpm run typecheck)
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/plugins/conversation-recovery.spec.ts e2e/tests/plugins/prompt-history-plugin.spec.ts e2e/tests/task/prompt-history-panel.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record existing unrelated CI or specification failures separately.
Do not remediate them under this work order without authorization.

## Dependencies

Task 02 creates the shared browser regression file and completes earlier shared
transport work. Task 01 supplies correct rebind grants.

## Risks

A resolved hydration promise is not necessarily a committed snapshot.
Browser and server clocks differ. Never broaden authorization to accommodate that difference.
A failed recovery must not repeatedly resubmit an expired cursor.


## Inputs

- [Plan and evidence](plan.md).
- [Recovery design](../../specs/plugins/system-design/conversation-recovery.md).
- [Existing requirement](../../specs/plugins/requirements/prompt-history-extraction-host.md).
- Source baseline: PR #3588, commit `792264a0736563d9597bda15cdda5ad97bd60f84`.
- Read the scoped AGENTS.md and TDD skill before implementation.

## Parallelism

`sequential`. Another agent can execute this work order after the user dispatches it.
Do not spawn agents from this work order.

## Results

RED evidence reproduced the expiry defect: an expired cursor and snapshot
token reached the continuation renewal endpoint, which correctly returned
non-retryable invalid_query and left loadMore unrecoverable.

The scope now detects expiry before renewal and performs one shared fresh
binding and subscription cycle. It fences old page and renewal responses,
notifies both message and turn readers, and continues the original paging
intent with the new cursor. Invalid queries and access failures remain
non-retryable. Terminal session removal retains committed rows and stops
later reads.

GREEN evidence:

- go test -tags fts5 ./internal/plugins ./internal/gateway/websocket: passed.
- pnpm exec vitest run lib/plugins/conversation-host.test.tsx lib/plugins/conversation-host-isolation.test.tsx: 2 files, 34 tests passed.
- pnpm run typecheck: passed.
- make -C apps/backend build: passed. The build reported only existing
  unsigned Darwin artifact warnings.
- pnpm run build:e2e: passed.
- make -C apps/backend e2e-plugin-package: passed.
- The combined Chromium E2E command passed all 5 tests: recovery, packaged
  plugin parity, and core prompt-history panel flows.
- The mobile-chrome prompt-history E2E passed its 1 test.
- python3 scripts/list-docs.py validate: passed.
- python3 scripts/lint-spec-files.py --all: passed.
- git diff --check: passed.

The checks do not constitute a full backend audit or the full PR review. The
broader durable transport scope decision remains separate.
