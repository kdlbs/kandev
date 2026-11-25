---
id: "02-access-setup"
title: "Separate host trust, access tests and optional enrollment"
status: done
wave: 2
depends_on:
  - "01-resource-views"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.1
  - AC-EXECUTORS-SSH-CONNECTIONS-001.2
  - AC-EXECUTORS-SSH-CONNECTIONS-002.2
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-003.1
  - AC-EXECUTORS-SSH-CONNECTIONS-003.2
  - AC-EXECUTORS-SSH-CONNECTIONS-003.3
  - AC-EXECUTORS-SSH-CONNECTIONS-003.4
  - AC-EXECUTORS-SSH-CONNECTIONS-003.5
  - AC-EXECUTORS-SSH-CONNECTIONS-003.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.2
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.10
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Task 02: Focus access setup and enrollment

## Summary

Give incomplete connections one clear next step. Make enrollment an explicit
optional operation and show its result without retaining the credential form.

## In scope

- Pure current-stamp next-step derivation; saved untrusted/unchecked/ready/error
  states, distinct Inspect/Trust/Test commands and dedicated diagnostics section.
- Explicit Install key entry, method-specific fields, public-key reveal/copy,
  verified-result replacement, persistent uncertain-outcome warnings.
- Active-section secret cleanup even when visited tabs force-mount; method,
  route-stamp, navigation, cancellation and late-response protections.
- Preserve existing confirmation, independently pinned trust, per-hop errors,
  one-time credentials, idempotency and separate executor readiness.
- Localization and desktop/phone unit, UI and real OpenSSH browser coverage.

## Out of scope

Backend changes, weaker host checks, automatic trust/enrollment/test, persistent
wizard progress, new retry behavior, live targets, global dialog/menu changes.

## Acceptance

1. UI-03 exposes only the relevant next-step form; a ready overview never shows
   enrollment credentials. Opening any view causes no probe or mutation.
2. Trust and enrollment retain explicit confirmations with complete required
   fingerprints/target. Authentication rejection is not diagnosed as a missing
   key without evidence. Uncertain authorization changes stay visible.
3. Verified enrollment replaces its credential form with result/Test action.
   Leaving or changing the form clears secrets even for previously visited
   tabs; late results cannot reopen the form or advance a different record.

## UI preview

UI-03 excerpt; [full state matrix](plan.md#ui-03-access-setup-enrollment-and-errors).
Structured preview follows the task's no-ASCII instruction.

| State | Primary content | Phone composition |
| --- | --- | --- |
| Untrusted | Target, next step Inspect, then explicit Trust | Focused page step, wrapping fingerprint |
| Trusted | Test SSH access; secondary Install key/Diagnostics | One primary action with labelled alternatives |
| Install requested | Access explanation, chosen method fields, confirmation | Inline full-page step, responsive confirmation |
| Verified | Result and Test; form removed | Same reachable result and command |
| Unknown/failed | Persistent warning and fresh-check path | Warning in main flow, no hidden rollback claim |

No stacked setup dialogs or multiple page scrollers. Touch actions are at least
44px. Full public keys are revealed on request and copied completely. ACs are
listed in frontmatter.

## Verification

Use `/tdd`; proposed `access-flow.test.tsx` and its desktop/phone browser specs
must fail before implementation. Preserve existing meaningful enrollment and
confirmation tests. Commands run from repo root, browser projects sequentially.
Real SSH uses a separate disposable test Docker daemon/fixtures, never the
running manual lab's daemon, main data, or a live host.

```bash
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx lib/api/domains/ssh-connections-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/ssh-connections e2e/tests/settings/ssh-access-setup.spec.ts e2e/tests/settings/mobile-ssh-access-setup.spec.ts e2e/tests/ssh/managed-enrollment.spec.ts e2e/tests/ssh/managed-connections.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --project containers tests/ssh/managed-enrollment.spec.ts tests/ssh/managed-connections.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Cover changed host, trusted-but-unchecked, authentication rejection, unavailable
agent/file/key, bastion failure, stale responses, busy submissions, refused
confirmation, installed-unverified/outcome-unknown/restart, and successful
replacement. Assert password/private-key absence from DOM after leaving a
visited panel and from navigation/browser storage, without printing inputs.
Use an active form's state as the lifecycle boundary, not CSS visibility.

## Files likely touched

Relative to `apps/web/`:

- `components/settings/ssh-connections/{connection-details,enrollment-panel,prerequisite-panel,confirmation}.tsx` and tests.
- Proposed local `connection-access.tsx`, `connection-diagnostics.tsx`,
  `access-flow.ts`, `access-flow.test.tsx` and focused child components.
- Task 01's presentation helper and tests where state is shared.
- Proposed `e2e/tests/settings/{ssh-access-setup,mobile-ssh-access-setup}.spec.ts`;
  existing `e2e/tests/ssh/{managed-enrollment,managed-connections}.spec.ts` and
  their affected scoped helpers.
- All affected settings locale catalogs, including generated Traditional Chinese.

## Dependencies

Task 01's detail and section navigation.

## Risks

Force-mounted hidden tabs retain secrets; stale success masks changed trust;
conditional forms hide uncertain results; convenience actions change command
semantics. Keep all guards with the existing domain service/API owner.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-connection-manager.md).
- [Interaction design](../../specs/executors/system-design/ssh-connection-management-ui.md):
  setup, enrollment and command/failure sections.
- [Service design](../../specs/executors/system-design/ssh-connection-manager.md):
  trust, enrollment and sanitized diagnostics.

## Results

Completed locally on 2026-10-26. Access and diagnostics are explicit views.
Installation opens a focused form only on request; closing/navigating clears
credentials, completion replaces the form and restores focus to Test, and
uncertain outcomes remain visible. Existing trust/enrollment confirmations and
separate executor-readiness messages remain intact. Fresh failed tests refresh
server state and carry route-stamp-scoped failure evidence instead of showing
an old successful snapshot.

RED: access lifecycle, post-enrollment focus, failed-probe reconciliation and
route-scoped failure-status assertions failed before their changes. GREEN:
52 unit cases in 11 files; typecheck; scoped lint; complete localization and
new-copy ratchet; seven desktop and three phone browser cases. Four distinct
real OpenSSH cases passed: managed key/reusable executor restart, direct
password enrollment, and bastion password/identity enrollment with restart and
preserved authorization entries. The two bastion cases passed on retry after
repairing disposable fixture networking.

The first container attempt stopped at stale Linux helpers and was not a test
result. Rebuilding remote agentctl/mock helpers resolved that. A fresh private
Docker daemon had firewall management disabled to avoid reconfiguring shared
Docker chains. Its bastion key-scan helper initially lacked outbound routing
for APK setup and timed out before product assertions. Scoped forwarding/NAT
rules for only its private fixture network fixed the setup; both bastion cases
then passed. Orphaned fixtures from the failed setup were removed explicitly.
No host verification or authentication assertion was weakened.

Receipts: /tmp/ssh-ux-access-{red,focus-red,focus-green,unit-verified,lint-final,type,i18n,ratchet}.log;
/tmp/ssh-ux-probe-status-{red,green}.log; /tmp/ssh-ux-access-{desktop,phone}.log;
/tmp/ssh-ux-access-openssh-retry.log and /tmp/ssh-ux-access-bastion-retry.log.
Private daemon/tooling metadata: /tmp/ssh-ux-docker-runtime.json. Keep it for
Task 03 adoption validation, then remove its owned fixtures/rules/daemon.
No publication, main-instance configuration or live remote SSH access change.
