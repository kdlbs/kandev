---
id: "03-contextual-integration"
title: "Integrate focused setup with Executors and existing return flows"
status: done
wave: 3
depends_on:
  - "01-resource-views"
  - "02-access-setup"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-002.7
  - AC-EXECUTORS-SSH-CONNECTIONS-002.8
  - AC-EXECUTORS-SSH-CONNECTIONS-004.1
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.7
  - AC-EXECUTORS-SSH-CONNECTIONS-004.8
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-005.2
  - AC-EXECUTORS-SSH-CONNECTIONS-005.3
  - AC-EXECUTORS-SSH-CONNECTIONS-005.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Task 03: Integrate focused setup with Executors

## Summary

Make SSH management look native from its Executors entry through contextual
creation and return. Preserve the reviewed executor, identity, bastion and
adoption semantics across the new focused views.

## In scope

- Move the gated `SSHConnectionsEntry` to `SettingsPageHeader.actions` on the
  Executors hub. Keep empty-profile discovery and existing profile sections.
- Quiet Create identity/Create bastion actions at their fields; save non-secret
  connection metadata through the existing coordinator before the detour.
- Compact return context, explicit Return/Use connection, focus and baseline
  reconciliation. Preserve adoption, nested returns and old bookmarks.
- Adapt all affected browser helpers and prove original direct/bastion adoption,
  disabled/admin boundaries, shared-save and desktop/phone behavior.
- Update public executor setup how-to and docs coverage, all affected catalogs,
  and final results in this package. Preserve prior package evidence/statuses.

## Out of scope

New continuation storage, automatic selection/testing/saving, new APIs,
changing feature defaults, CI/review fixes, resolving merge conflicts, running
instance changes, committing or publishing.

## Acceptance

1. UI-01 Executors header action is coherent on desktop and phone and remains
   discoverable with zero profiles. Member/disabled gates and old links remain.
2. UI-04 create/cancel/Back/return preserves non-secret drafts and outer context.
   Only explicit selection changes the executor draft. Return invalidates its
   checks; final check and Save remain separate, with stale baselines refused.
3. Desktop/phone, direct/bastion adoption, disabled/auth, localization and docs
   checks pass. Results distinguish UI fixtures from real SSH evidence, and
   only this package's completed work is marked done.

## UI preview

UI-01 entry and UI-04 excerpt; [full previews](plan.md#ui-previews).
Structured preview follows the task's no-ASCII instruction.

| Step | Desktop | Phone |
| --- | --- | --- |
| Executors | Title/description with secondary SSH connections header action | Same shared header, action on its own touch row |
| Connection fields | Identity picker + quiet Create identity; optional bastion equivalent | Picker drawer; creation navigates to focused form |
| Related form | Create/Cancel; preserve originating details | Focused form; Back/Cancel clears secret inputs |
| Return | Restored draft; explicit Return or Use connection when eligible | Same wrapped 44px actions and focus return |
| Executor | New executor check, then shared Save changes | Same deliberate actions, safe-area Save |

No separate navigation-only vertical band on the hub. No automatic auth, trust,
enrollment or save during return. Labels illustrate required hierarchy. The
ACs are listed in frontmatter.

## Verification

Use `/tdd` for changed entry, route and continuation logic. Update the existing
specs/helpers; do not weaken assertions to accommodate layout changes. Run from
repo root. Projects are sequential and use their default worker budgets. A
fresh worktree requires `pnpm --dir apps install --frozen-lockfile` first.
Use a separate disposable Docker daemon for container scenarios.

```bash
(cd apps/web && pnpm exec vitest run app/settings/executors/page.test.tsx components/settings/ssh-connections lib/settings/ssh-connection-settings-routes.test.ts lib/settings-discovery/catalog.test.ts components/settings/settings-breadcrumbs.test.ts components/settings/settings-save-provider.test.tsx components/settings/settings-save-exclusive.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/settings/executors/page.tsx app/settings/executors/page.test.tsx components/settings/ssh-connections lib/settings/ssh-connection-settings-routes.ts lib/settings/ssh-connection-settings-routes.test.ts e2e/helpers/ssh-context-setup.ts e2e/helpers/ssh-navigation.ts e2e/tests/settings/ssh-profile-connection-link.spec.ts e2e/tests/settings/mobile-ssh-profile-connection-link.spec.ts e2e/tests/ssh/managed-adoption-browser.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-navigation.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-profile-connection-link.spec.ts tests/settings/ssh-management-layout.spec.ts tests/settings/ssh-connections-disabled.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-connections.spec.ts tests/settings/mobile-ssh-profile-connection-link.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project auth tests/auth/ssh-connections-permissions.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --project containers tests/ssh/managed-adoption-browser.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Exercise create/cancel and failed save; Back/Forward and reload; identity/bastion
nested returns; 767/768px; coarse tablet controls; selection versus return;
concurrent editor changes; missing/deleted connection; gates; final executor
check plus shared Save; unsupported chained adoption with original profile intact.
Record the exact files changed and all executed checks/counts. Update only
current outcomes; historical evidence stays dated. Run changed-file formatting
and lint for any additional files introduced by implementation.

## Files likely touched

Relative to `apps/web/` unless prefixed `docs/`:

- `app/settings/executors/page.tsx`, `page.test.tsx`.
- `components/settings/ssh-connections/{connection-details,identity-form,setup-return-actions,setup-continuation,managed-binding,adoption}.tsx`, associated tests, and `use-manager-navigation.ts`.
- `lib/settings/ssh-connection-settings-routes.ts`, `.test.ts`.
- Existing `e2e/helpers/{ssh-context-setup,ssh-navigation,ssh-identities}.ts` and
  settings/adoption specs named above. Preserve task 01/02 layout coverage.
- Affected locale catalogs, including pseudo and generated Traditional Chinese.
- `docs/public/executors.md`, `docs/public/coverage.json` if its mapping changes.
- Current package/spec statuses and linked implementation records only as needed.

## Dependencies

Tasks 01 and 02. Preserve `SSHSetupContinuationProvider` outside pathname-keyed
save ownership and preserve current route helper allowlists.

## Risks

Header integration hides the entry for empty lists; contextual creation silently
saves more than intended; returning applies stale selection or loses drafts;
old query fields disappear; old completion evidence is mistaken for this revision.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-connection-manager.md).
- [Interaction design](../../specs/executors/system-design/ssh-connection-management-ui.md):
  navigation, continuity, mobile and failure sections.
- [Prior package](../ssh-connections-executor-navigation/plan.md) and its completed
  continuation work order, plus existing adoption/permission browser fixtures.

## Results

Completed locally on 2026-10-26 in the primary session. No subagents or model
switch were used. SSH connections now uses the Executors header action. Related
identity/bastion creation uses quiet field actions with draft-save help, and
retains explicit selection, return, test and save steps. Saved-item actions use
the existing responsive confirmation. Public setup guidance matches the new
Overview, Access, Diagnostics and Edit flow.

RED/GREEN evidence: the mixed `new=1&id=existing` bookmark opened the old record
before the creation guard; the new regression passes with an empty draft. The
delete-menu browser case failed because menu dismissal also dismissed the
confirmation; opening the confirmation after menu close now preserves cancel,
delete and focus return on desktop and phone. A cancellation URL regression
also failed before clearing the pending identity-selection parameter.

Final checks passed:

- Focused Vitest command above, plus `use-ssh-connections.test.tsx` and
  `ssh-connections-api.test.ts`: 130 tests in 17 files.
- Web typecheck and rich-output catalog check; changed/new TS/TSX ESLint with
  zero warnings; changed/new TS/TSX Prettier check; complete localization and
  new-copy ratchet. English, six translations and pseudo stay in sync.
- Chromium: 21 cases from all five listed desktop specs plus
  `ssh-access-setup.spec.ts`. Phone: 13 cases from the four listed phone specs
  plus `mobile-ssh-access-setup.spec.ts`. Includes breakpoint/coarse-tablet
  geometry, touch confirmation, light/dark, pseudo locale, focus, continuation,
  stale-editor rejection and zero document overflow.
- Real OpenSSH adoption: all eight `managed-adoption-browser` cases passed.
  Desktop and phone each cover direct, bastion, unpinned and unsupported-chain
  cases, including cancellation, concurrent edits, preserved pins and reload.
  Administrator/member API and UI boundary: one auth case passed. These 43
  final browser cases distinguish mocked access UI from real SSH assertions.
- Public-doc validator tests: 63 passed; 54 published pages validated.
  Specification tests: 36 passed; 393 decisions and 1674 specifications
  validated; specification lint, local delivery-coverage preflight (all three
  work orders), and diff whitespace checks passed.

The first adoption attempt failed at a locator that matched both the original
executor fingerprint and the adoption-preview fingerprint. Scoping the locator
to the preview retained the full-value assertion; the complete eight-case retry
passed. No product trust, authentication, or concurrency assertion was removed.
The auth fixture now verifies the saved overview and explicit Edit action.

Real tests used the private disposable daemon recorded in
`/tmp/ssh-ux-docker-completed.json`, never the main daemon or the user's lab.
Fixtures removed all containers and custom networks. Cleanup stopped that daemon,
removed its temporary data root, bridge and remaining owned firewall rule.
`/tmp/ssh-ux-context-cleanup.log` records cleanup. The main instance remained
listening at `100.101.37.39:9998` (PID 1167); the prior lab at port 43735 was
already stopped when checked and was not refreshed.

Receipts: `/tmp/ssh-ux-context-{unit-final,type-final,lint-final,format-final,i18n,ratchet,desktop,phone,adoption,adoption-retry,auth}.log`;
`/tmp/ssh-ux-context-{cancel-red,doc-tests,docs,catalog,spec-tests,spec-lint,cleanup}.log`;
`/tmp/ssh-ux-context-delivery-preflight.json`. Browser details:
`/tmp/kandev-run.e2e.{51FMF0fY,rkeaZsMr,PIF4oHff,o9yODANH,v4GOOKWj}.log`.
The first adoption log is a failed attempt; `o9yODANH` is the green retry.
Desktop/phone overview and verified-access screenshots were preserved and
visually inspected under `/tmp/ssh-ux-evidence/context-{desktop,phone}/`.

All three work orders are done. The owning requirements are active and both
paired designs are current. The service design now identifies its historical
integration seams and uses the implemented runtime admission/dial names.
No backend source, shipped defaults, live SSH authorization, running-instance
configuration, PR, commit, push or merge-conflict remediation changed.
