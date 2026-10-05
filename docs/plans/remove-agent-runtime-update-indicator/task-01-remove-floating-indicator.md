---
id: "01-remove-floating-indicator"
title: "Remove the floating agent runtime update indicator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.2
  - AC-AGENTS-RUNTIME-NOTIFY-001.3
  - AC-AGENTS-RUNTIME-NOTIFY-001.6
  - AC-AGENTS-RUNTIME-NOTIFY-001.7
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---

# Task 01: Remove the Floating Agent Runtime Update Indicator

## Summary

Remove the floating count/control on desktop and phone. Preserve the mounted
notification/status bridge and all Settings runtime management paths. Deliver
regression proof and documentation together using TDD.

## In scope

- Bridge rendering removal, unused presentation imports and locale key cleanup.
- New bridge unit test; shared desktop/phone helper changes and rendered captures.
- Public guide navigation correction; this package's final results and spec lifecycle.

## Out of scope

- Backend scheduling, permissions, notifications, runtime mutation or policies.
- Archive changes, replacement controls and new toggles.

## Acceptance

1. No floating runtime indicator renders with available updates on desktop or phone.
2. Notification delivery, status refresh/cleanup, Settings access, fragment reveal
   and existing manual/policy/recovery flows pass their focused regressions.
3. Public guide and locale catalogs match the removal; targeted checks and
   desktop/phone screenshots are recorded before marking this order done.

## ASCII UI preview

See the [full preview](plan.md#ascii-ui-preview).

### UI-01: Desktop app view, newer runtimes available

```text
Before: [Download: 2 agent runtime updates]  [Archive]
After:                                     [Archive]
```

### UI-02: Phone app view, newer runtimes available

```text
[App content]
[Existing phone navigation]
```

The floating runtime update control is absent on both surfaces. Existing app
content, archive and navigation retain their positions. Settings > Agents is
the persistent management entry; a runtime toast's Review runtime link still
opens its row. These are structural requirements for
AC-AGENTS-RUNTIME-NOTIFY-001.7 and 001.6; spacing is illustrative. No new overlay,
scroll owner, touch target or loading/error presentation is introduced. The
nearest shipped mobile precedents are SettingsPageTemplate, InstalledAgentCard
and AgentRuntimeUpdateSurface's existing version drawer. Existing page scrolling,
safe areas and 44px phone settings/notification targets remain authoritative.

## Verification

```bash
# From the repository root. Bootstrap dependencies once in this worktree.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/update-available-toast-bridge.test.tsx hooks/use-update-available-toast.test.ts hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx)
(cd apps/web && pnpm exec eslint components/update-available-toast-bridge.tsx components/update-available-toast-bridge.test.tsx e2e/tests/settings/agent-runtime-notifications-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the unit regression red before editing the bridge. E2E projects run
sequentially with fresh builds. Remove all unused indicator keys consistently;
run `(cd apps/web && pnpm run i18n:pseudo)` after source-locale edits.

## Files likely touched

- `apps/web/components/update-available-toast-bridge.tsx`
- `apps/web/components/update-available-toast-bridge.test.tsx` (new)
- `apps/web/e2e/tests/settings/agent-runtime-notifications-helpers.ts`
- `apps/web/e2e/tests/settings/agent-runtime-notifications.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-notifications.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json`
- `docs/public/agents-and-profiles.md`
- `docs/specs/agents/{requirements,system-design}/runtime-update-notifications.md`
- This plan and work order for statuses/results.

## Dependencies

None. Read `apps/web/AGENTS.md`, the linked requirement/design, `/tdd`, `/e2e`
and `/mobile-parity` before implementation.

## Risks

Keep the bridge mounted and avoid deleting its notification/status effects.
Retain positive Settings/notification navigation evidence when deleting the
indicator click, including phone touch and narrow fine-pointer assertions.

## Parallelism

`sequential`

## Inputs

- [Runtime notification requirements](../../specs/agents/requirements/runtime-update-notifications.md), REQ-AGENTS-RUNTIME-NOTIFY-001.
- [Paired design](../../specs/agents/system-design/runtime-update-notifications.md), Responsive UI.
- Existing bridge, toast/status hook tests and shared notification E2E helper.
- [Original package](../agent-runtime-notifications/plan.md) and [compact settings package](../agent-runtime-settings-compact/plan.md) for historical delivery context.

## Results

Completed on 2026-10-02. All Verification commands passed from their documented
working directories:

- Dependency bootstrap: frozen workspace installation succeeded.
- Vitest: initial old-code regression produced 3 failures/4 passes; final three
  suites passed all 18 tests. Typed notification fixtures include occurrence IDs.
- Changed-file ESLint and final TypeScript typecheck passed.
- i18n checks passed for all locales; pseudo regenerated after removing the
  obsolete plural keys.
- Fresh-build desktop E2E: 5 passed, including 390px and 767/768px cases.
- Fresh-build mobile E2E: 2 passed, including retained fallback management.
- Public validators: 62 tests passed and 47 published pages validated.
- Specification catalog: 342 decisions and 1304 specs validated; all spec lint
  and diff whitespace checks passed.
- Documentation coverage preflight: planned bridge change and complete linked
  work order accepted with `status: covered` and no errors.

Fresh screenshots were captured in additional focused runs against the unchanged
fresh build: `pnpm e2e:run --host --no-build --project chromium
 tests/settings/agent-runtime-notifications.spec.ts --grep 'runtime notices, saved consent'`
and `pnpm e2e:run --host --no-build --project mobile-chrome
 tests/settings/mobile-agent-runtime-notifications.spec.ts --grep 'phone runtime notices'`
(each passed 1 test, with CAPTURE_PR_ASSETS=true). These focused captures avoid
later tests in the same file replacing earlier manifest entries. The desktop
manifest was preserved before the phone runner cleared the asset directory;
the combined manifest has 12 current entries, all resolving to existing files.
The two `runtime-updates-no-floating-indicator` captures were visually compared
with UI-01/UI-02: app navigation remains visible and the floating control is
absent. Selected PR PNGs were compressed with pngquant. No source changes
occurred between the final builds and captures. Assets remain outside the PR
source branch. No unresolved implementation risks remain.

The final behavior matches the linked requirements/design. Their statuses are
active/current and this package is implemented. PR review/merge evidence is
tracked in the Kandev task plan.
