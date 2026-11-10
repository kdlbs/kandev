---
id: "01-preserve-drafts"
title: "Preserve later notification drafts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.12
system_design:
  - ../../specs/platform/system-design/notification-settings-atomicity.md
---

# Preserve later notification drafts

Status: done

## Contract

Implement [AC-PLATFORM-NOTIFICATIONS-001.12](../../specs/platform/requirements/notifications.md)
under the [mounted acknowledgement design](../../specs/platform/system-design/notification-settings-atomicity.md#mounted-save-acknowledgement).
[Plan](plan.md) owns parity, exclusions and delivery barriers. Dependencies: none.

## Owned files and scope

- `apps/web/components/settings/notifications-settings-actions.ts`
- `apps/web/components/settings/notification-save-reconciliation.ts`.
- `apps/web/components/settings/notification-save-drafts.test.tsx`
- Focused desktop/phone notification-save browser specs and shared scenario.
- Existing owning Platform requirement/design; this plan and work order.

No backend/API, initial GET, cache authority, delivery policy, sibling diff-mode,
operator runtime or shared seed edits. No new public UI copy. Same mounted draft
semantics on unchanged desktop table and phone stacked controls (state-only
exception to composition previews; see plan).

## Acceptance

1. Real hooks, provider/store, API/fetchJson and coordinator with only HTTP held
   prove RED for later additions and undo, with unchanged success controls.
   Success advances canonical saved state but preserves later fields, raw
   buffers, deletions and create-form identity.
2. Second explicit Save persists remaining changes; Reset restores accepted
   canonical state. Confirmed failure retains draft/errors. Normalized responses
   must not discard independent later choices or leave unchanged submissions dirty.
3. Targeted managed desktop and native-phone browser scenarios hold only causal
   HTTP, use disposable real backend data, restore fixture policy and send no
   notifications. Every handle reaches terminal; shared seed stays unchanged.

## Exact verification

From `apps/web` (Node24, `NODE_OPTIONS=--max-old-space-size=8192`):

```bash
pnpm exec vitest run components/settings/notification-save-drafts.test.tsx components/settings/notifications-settings-actions.test.tsx components/settings/notifications-settings.test.tsx components/settings/settings-save-provider.test.tsx
pnpm exec eslint --max-warnings 0 components/settings/notifications-settings-actions.ts components/settings/notification-save-reconciliation.ts components/settings/notification-save-drafts.test.tsx e2e/tests/settings/notification-save-drafts-helpers.ts e2e/tests/settings/notification-save-drafts.spec.ts e2e/tests/settings/mobile-notification-save-drafts.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium -- tests/settings/notification-save-drafts.spec.ts --retries=0
pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/settings/mobile-notification-save-drafts.spec.ts --retries=0
```

Include any new helper and E2E paths in scoped lint. From repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
git diff --check
```

Refresh open-PR inventory and exact PR1512 target patch before handoff. Inspect
all changed/new files, normal hooks and exact committed head. Stop before push.

## Results

Implemented notification-specific acknowledgement reconciliation. No public docs
change: existing manual Save semantics are restored, with no new copy, navigation,
commands or public API. The public Apprise guide's independent-provider and
ambiguous-transport warnings remain accurate.

- Permanent RED at the base: 16 cases, 11 expected behavioral failures and five
  passing controls. Initial setup/matcher corrections are retained in raw logs
  and are not counted as RED evidence.
- GREEN: the four specified Vitest files pass, 62 tests. Coverage includes real
  hooks/provider/store/API/fetchJson and mounted notification page, event controls,
  Apprise card and coordinator; only the actual fetch transport is held.
- Managed fresh production builds: desktop Chromium 1/1 and native Pixel 5
  mobile-chrome 1/1 pass, one worker/shard, retries zero. Only one causal PATCH
  response is held after real backend persistence. Both tests prove second Save,
  Reset and reload. The phone test taps shipped 44px label targets. Browser RED
  is owned by the deterministic real-consumer integration regression, not an
  invented live incident.
- Disposable fixtures disable provider policy during the scenario, create one
  disabled test provider, delete it and restore prior enabled state in finally.
  No test-notification endpoint or task agent is invoked. Seed checkout HEAD and
  porcelain status are asserted unchanged. No runtime task filesystem mutation
  occurs; the fixture's worktree executor profile is explicitly verified.
- Scoped lint, Node24 typecheck (8GiB heap cap), i18n check, catalog validation,
  full specification lint, specification-linter tests (36), public-doc validator
  (54 pages) and its tests (63), documentation-coverage preflight and diff checks
  pass. No catalog/copy changes were needed.

All raw command/hook/process receipts are outside the checkout in the task's
`receipts/` directory. Normal-hook commit and exact-head handoff are recorded
there; publication remains blocked until independent parent validation and
separate same-session delivery authorization. Partial batch progress and
ambiguous transport recovery remain existing limitations.

Integration base: `edafc7900de0328a9997401f11a0d76cb8c548d3` includes sibling
PR4680. The isolated, uncommitted branch advanced with a fast-forward only;
notification production bytes stayed identical. Focused checks and managed
browser builds were refreshed on that integrated base before the commit.
