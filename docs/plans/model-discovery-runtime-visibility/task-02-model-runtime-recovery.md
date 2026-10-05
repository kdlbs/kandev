---
id: "02-model-runtime-recovery"
title: "Show runtime context and connect recovery"
status: done
wave: 2
depends_on:
  - "01-runtime-observations"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.1
  - AC-AGENTS-RUNTIME-UPDATES-003.2
  - AC-AGENTS-RUNTIME-UPDATES-003.3
  - AC-AGENTS-RUNTIME-UPDATES-003.4
  - AC-AGENTS-RUNTIME-UPDATES-003.5
  - AC-AGENTS-RUNTIME-UPDATES-003.6
  - AC-AGENTS-RUNTIME-UPDATES-003.7
  - AC-AGENTS-RUNTIME-UPDATES-003.8
system_design:
  - ../../specs/agents/system-design/runtime-model-discovery.md
---

# Task 02: Show runtime context and connect recovery

## Delivery scope revision

This completed work order records the original runtime panel delivery.
On 2026-10-05, the user removed runtime presentation from profile pages.
[Task 03](task-03-remove-profile-runtime-panel.md) supersedes the panel, its actions, locale copy, and presentation checks.
The activation refresh behavior remains current. Read the revised requirement and system design for the current contract.

## Summary

Show the matching host-runtime observation beneath profile model controls and expose existing explicit recovery.
After successful activation, refresh the current profile draft once and preserve model selections.
Document this flow and prove desktop and phone behavior.

## In scope

- Optional web response types, hook projection, and a small localized `ProfileRuntimeInfo` component.
- Managed bridge versus bundled SDK/CLI and external provider source labels, with explicit unknown observations.
- Shared update status and direct runtime-settings target or trusted provider guidance.
- Trusted manual guidance for an external primary runtime, without managed-only release wording.
- Existing Settings navigation coordination for dirty drafts, with no silent discard.
- Matching succeeded-job refresh, mount baselines, job deduplication, and late-result rejection.
- Subscribe the profile runtime status panel to its matching terminal update job while reusing the shared status cache and refresh hook.
- Localized copy in all seven languages, mobile control geometry, E2E, and public guidance.

## Out of scope

New update engines, embedded duplicate version dialogs, automatic consent changes, guessed model access, remote inspection, and profile schema changes are excluded.

## Acceptance

1. Desktop and phone model settings show accurate source/version labels and reachable managed or external recovery, including unknown/offline states.
2. A new matching successful update refreshes current unsaved launch settings once. Failed, duplicate, historical, and unrelated jobs preserve the prior snapshot.
3. The updated catalog and observation stay correlated, selections remain unchanged, and public guidance describes host scope and external ownership.

## ASCII UI preview

UI-01 and UI-02 match the [combined plan previews](plan.md#ascii-ui-preview).

```text
Desktop UI-01:
Model [Selected v]  Mode [v]  [Refresh]
Host runtime: bridge 1.11.0 observed / 1.11.0 configured. Managed
Codex 0.153.4. Bundled
[Manage bridge version] [Provider guidance]

Phone UI-01:
Model [Selected v]
[             Refresh             ]
ACP bridge: 1.11.0 observed
Configured: 1.11.0. Managed
Codex: 0.153.4. Bundled
[      Manage bridge version      ]
[        Provider guidance        ]

UI-02:
Codex: 0.160.0. External
Bridge updates do not update external Codex.
Unknown variant: Codex version unknown.
```

Use the current page scroller, wrapping rows, and visible actions.
The destination reuses the existing desktop dialog and phone drawer, with their own internal scrolling and safe-area behavior.
The nearest shipped examples are profile capability controls, compact runtime policy rows, and `AgentRuntimeUpdateSurface`.
Direct navigation suits persistent runtime settings. Temporary version selection stays in the existing drawer.
Do not add a nested surface or require hover for recovery.
Phone and coarse-pointer actions have at least 44px targets. Desktop fine-pointer actions remain 28px.
The grouping and source distinctions are required. Example copy and versions are illustrative.
These views cover AC-AGENTS-RUNTIME-UPDATES-003.1 through 003.7.

## TDD and regression evidence

First add a hook test named `refreshes current draft once after matching successful update`.
Start with a completed baseline probe, change an unsaved launch setting, and deliver a new matching succeeded job.
Assert a fresh profile request containing that draft. Current code fails because it does not observe terminal update jobs.
Record this behavioral RED before changing hook logic.

Add tests for duplicated completion, historical terminal state on mount, another agent, failed candidate, and interrupted discovery.
Delay an old probe across activation and require that its versions and models cannot replace the new matching response.
Refresh failure after activation must not rewrite the update outcome or selected model.

Component tests cover configured/observed disagreement, external-versus-bundled identity, unknown observations, offline release status, and recovery links.
Do not compare translated labels or independently classify update operations.

Extend existing desktop and mobile profile-discovery E2E with deterministic version observations and existing update-preview/job helpers.
Keep the profile open while another tab uses the existing runtime update surface.
Arm causal HTTP/WS observers before approval. Require one new profile refresh with current draft values after success.
Assert new model choices, distinct component versions, retained selection, and unchanged saved profile data.
For a failed candidate, assert the prior catalog and observation remain visible with retryable update feedback.
Follow the management link and require the existing runtime disclosure to open.
Verify dirty navigation requires the existing save/discard decision and does not silently discard edits.

On phones, assert touch target bounds, long-source wrapping, document overflow, and drawer containment.
Repeat the action-size assertion at a narrow fine-pointer viewport below 768px.
Capture a rendered phone screenshot and compare its structure with UI-01 and UI-02.
Keep the existing mobile profile refresh and runtime update flows passing.

Update `docs/public/agents-and-profiles.md` as a how-to guide.
Explain observed versus configured versions, bundled versus external components, host scope, explicit bridge updates, and refresh after success.
State that runtime versions can affect discovery but do not prove account access to missing models.
Search root README and screenshot catalog wording. Change them only if their current statements become inaccurate.

## Verification

Run from the repository root. Install workspace dependencies once before the first package command in a fresh worktree.
The managed E2E runner rebuilds the production web/backend artifacts and owns teardown.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-profile-model-capabilities.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx components/settings/profile-runtime-info.test.tsx components/settings/profile-capability-helpers.test.tsx components/settings/agent-runtime-update-control.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/profile-runtime-info.tsx components/settings/profile-runtime-info.test.tsx components/settings/profile-capabilities-row.tsx components/settings/profile-capability-helpers.tsx hooks/domains/settings/use-profile-model-capabilities.ts lib/types/http-agents.ts e2e/helpers/profile-capability-discovery.ts e2e/tests/settings/profile-capability-discovery.spec.ts e2e/tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/profile-capability-discovery.spec.ts tests/settings/agent-runtime-update.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-profile-capability-discovery.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If shared E2E helpers change, include their ESLint paths and run every affected spec listed here.
Do not overlap the desktop and mobile runner commands.

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/domains/settings/use-profile-model-capabilities.ts` and sibling tests
- `apps/web/components/settings/profile-capability-helpers.tsx` and sibling tests
- `apps/web/components/settings/profile-capabilities-row.tsx`
- `apps/web/components/settings/profile-runtime-info.tsx` and sibling tests (new)
- `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts`
- `apps/web/e2e/tests/settings/mobile-profile-capability-discovery.spec.ts`
- `apps/web/e2e/helpers/profile-capability-discovery.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts` if shared fixtures need extending
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/agents.json`
- `docs/public/agents-and-profiles.md`
- `README.md` or `docs/screenshots.md` only if existing guidance needs correction
- This plan and both work-order result sections

## Dependencies

Task 01 supplies the optional runtime observation contract.
Reuse the current target registry, store update jobs, status hook, updater, and responsive update surface.

## Risks

The global candidate snapshot can differ from profile discovery. Always use the current complete draft after activation.
Navigation with unsaved settings must retain the existing save barrier. Do not bypass it to make the management link appear seamless.
Test mocks must deliver terminal events and new profile responses rather than mutate only an agent-wide catalog.

## Parallelism

`sequential`

## Inputs

- [Runtime requirement](../../specs/agents/requirements/runtime-updates.md), REQ-AGENTS-RUNTIME-UPDATES-003.
- [Runtime model discovery design](../../specs/agents/system-design/runtime-model-discovery.md), Snapshot flow through Failure and security.
- Existing desktop/phone profile-discovery specs and runtime-update helpers.
- `/mobile-parity`, `/e2e`, `/docs-maintainer`, `/simple-english`, and web `AGENTS.md` guidance.

## Results

Completed 2026-10-04.

- Added the typed web projection, a localized runtime details panel, and matching successful-update refresh behavior. A stale profile keeps the last observation visibly marked stale while its catalog remains unavailable until refreshed.
- Added hook and component coverage. The focused Vitest suite passed 21 tests; web typecheck and targeted ESLint passed.
- `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet` passed for all seven language catalogs.
- Managed desktop E2E passed 18 tests. Managed phone E2E passed 8 tests, including touch target, narrow fine-pointer, overflow, drawer, and second-tab refresh checks.
- Updated public profile guidance for host scope, observed versus configured versions, bundled versus external ownership, recovery, and account-access limits. Public documentation tests and validation passed.
- Specification catalog validation, spec lint, and `git diff --check` passed.
- Follow-up review regressions cover external OpenCode recovery and release-status suppression, shared status refresh after successful updates, one-refresh deduplication, and retention of an available status after failed updates. The focused profile/status hook suite passed 11 tests; web typecheck, targeted ESLint, locale checks, and the new-code i18n ratchet passed. Desktop E2E passed 18 tests and mobile E2E passed 9 tests, including second-tab status transitions and the touch-sized external-runtime guidance action.
- A command-prefixed launch with a trusted Kandev-managed fallback keeps its configured version and settings recovery action while omitting managed release-status wording for the unknown active source. The profile runtime component regression passes.
- The focused mobile profile-discovery E2E suite passed 5 tests, including the unknown prefixed-runtime recovery link, its touch target, and no horizontal overflow.
- Review follow-up limits managed update controls to administrators while keeping trusted external guidance available, reuses shared localized runtime labels, and distinguishes unknown from unavailable details in Portuguese. Profile component coverage verifies refreshed status changes after both successful and failed jobs; desktop and mobile second-tab E2E flows assert the updated status message. An extra mobile browser context now receives the backend API port before navigation.
- Profile form and model-option test renderers now mount the application store required by the runtime status subscription.
