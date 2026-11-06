---
created: 2026-10-22
status: done
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/notification-settings-atomicity.md
legacy_specs: []
---

# Preserve notification drafts during Save

Status: done

## Outcome and ownership

Repair the confirmed mounted notification Save race: `useSaveRequest` currently
replaces live providers and raw Apprise buffers unconditionally with a captured
request snapshot. A later checkbox edit is lost after acknowledgement. The
parent's real-hook/StateProvider/API diagnostic has two behavioral failures and
one unchanged success control at main `8c474af8ef8c9ee799a228ab075fa7bfc26d907f`.

Owner: [REQ-PLATFORM-NOTIFICATIONS-001, AC-PLATFORM-NOTIFICATIONS-001.12](../../specs/platform/requirements/notifications.md).
Design: [notification settings atomicity](../../specs/platform/system-design/notification-settings-atomicity.md#mounted-save-acknowledgement).
The UI manual-Save contract is shared context, not a second owner.

## Work package

1. [Preserve later drafts](task-01-preserve-drafts.md) (done). No dependencies.

The user explicitly authorized this package and same-turn TDD. No delegation.
PR1512 remains a separate Query-cache owner migration; refreshed inventory has
69 open PRs and its exact head is `a8655ad92faa050a68140a904500fc99ebc0a04b`.
Its actions patch still unconditionally replaces drafts. Recheck before handoff.

## Surface and parity

Pure state/data reconciliation in unchanged shipped controls. The desktop table
and phone stacked event list in `notification-events-table.tsx` share the same
hook/actions. No rendered structure, navigation, touch or scrolling changes, so
no new ASCII composition is needed. Verify both shipped surfaces with managed
browser tests; use `.tap()` on the phone's existing 44px label. No new mobile
entry is introduced. Component integration is the behavioral RED gate.

## Verification and limits

See the work order for exact commands. Keep receipts outside the checkout.
Use one worker/shard, retries zero, normal guards, Node24 with an 8GiB heap cap,
GOMAXPROCS=2 and GOFLAGS=-p=2. No full-suite overlap. Only transport is deferred;
no provider sends, live operator instance or shared seed mutation.

Exclude initial GET recovery, backend/API changes, Query migration, server write
ordering, new durable authority and batch/exactly-once/ambiguous-outcome promises.
Partial batch progress on failure remains a limitation. No ADR is needed because
the existing owner and boundaries remain intact. Public docs assessment is
required; no new user copy or public contract is planned.

## Delivery checkpoint

Normal active pre-commit and commit-msg hooks, clean worktree and every owned
handle terminal, then queued `phase=ready_for_parent_validation` to controller.
STOP before push/PR. Parent independently reviews the diff and reruns focused
checks, then grants separate same-session delivery authorization. Controller
messages must explicitly use `delivery_mode="interrupt"`; upward notifications
use `delivery_mode="queued"`. Do not cancel foreign processes or GitHub jobs.

## Verification results

The [work-order results](task-01-preserve-drafts.md#results) record the completed
matrix: 16 permanent regression cases (11 behavioral RED, five controls), 62
GREEN focused tests, managed desktop and native phone browser checks (one test
each, one worker/shard, zero retries), Node24 typecheck, scoped lint, i18n and
documentation checks. No public guide or locale copy changed. External receipts
retain raw logs, commands, process outcomes and normal hook evidence. Work is
complete locally; publication awaits the independent parent checkpoint.

Integration base: `edafc7900de0328a9997401f11a0d76cb8c548d3` includes sibling
PR4680. The isolated, uncommitted branch advanced with a fast-forward only;
notification production bytes stayed identical. Focused checks and managed
browser builds were refreshed on that integrated base before the commit.
