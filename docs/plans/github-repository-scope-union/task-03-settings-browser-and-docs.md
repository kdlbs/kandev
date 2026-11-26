---
id: "03-settings-browser-and-docs"
title: "Combined settings with desktop and phone proof"
status: done
wave: 3
depends_on:
  - "02-existing-scope-consumers"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.9
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.11
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.12
system_design:
  - ../../specs/integrations/system-design/github-repository-scope.md
---

# Task 03: Combined settings with desktop and phone proof

## Outcome and dependencies

Replace the Select with a Switch and both existing pickers. Complete focused
browser evidence and a short documentation tail in this same work order.
Work orders 01-02 must pass. Read web guidance, `/mobile-parity`, and `/e2e`.
Reuse the landed picker design, including its scope-editor discovery source.

## Owned scope

- `apps/web/components/github/github-repo-scope-section.tsx` and its tests:
  toggle, both lists, payload, shared save/discard, retry, and pending-edit guards.
- `apps/web/lib/types/github.ts`, `lib/github/repo-scope.ts` and its tests,
  `components/github/repo-filter-selector.tsx` and its tests: canonical mode
  readers, union membership, owner visibility, and wildcard eligibility.
- Preserve `repositorySource="scope-editor"` on the scope repository picker.
  Its endpoint is `/api/v1/github/workspace-settings/repositories`. Ordinary
  `/api/v1/github/repos` stays scope-filtered. Scope organizations receive no
  `isOrgAllowed` filter. Watch organizations retain that predicate.
- Reuse existing localized GitHub labels and regenerate pseudo; add no new copy.
- `apps/backend/internal/settingscatalog/defaults.go`: update the existing
  string descriptor's description. It has no enum today. Regenerate the catalog
  snapshot without adding a new enum-discovery mechanism.
- Desktop `e2e/tests/integrations/github-repository-scope-union.spec.ts` and one
  `e2e/tests/mobile-github-repository-scope-union.spec.ts`.
- Existing `e2e/tests/integrations/github-workspace-settings.spec.ts`: replace
  canonical GET expectations on `orgs`/`repos` with `selected`. Keep separate
  tests that deliberately send legacy requests.
- Audit `e2e/helpers/api-client.ts`,
  `e2e/tests/automations-pr-merged-trigger.spec.ts`, and
  `e2e/tests/task/create-task-remote-repo.spec.ts`. They contain no mode literals
  at the rebased base. Avoid unrelated edits. If changed, run their focused suites.
- Reconcile the landed picker requirement/design's scope wording and criteria
  .3, .6, .9, .10. Link the scope contract. Preserve historical picker results
  and completed task statuses in its companion plan.
- `docs/public/integrations.md`: a short Repository Scope how-to. Add a concise
  amendment to ADR 0030 and preserve ADR 0047's identity ownership boundaries.

## Acceptance

1. Component/predicate tests fail first, then prove mixed save, All on/off,
   empty denial, Discard, retry, and preservation of edits during a pending save.
   Watch repository picks match either grant. Owner wildcards require an owner grant.
2. Desktop browser evidence proves mixed save/readback, reload, and canonical
   payload. Exact-only scope can add an organization through the unfiltered
   editor. One mobile spec proves mixed selection and save/reload using the
   existing accessible touch controls and contained pickers.
3. Locales, catalog description/snapshot, canonical fixtures, public docs, and
   the ADR amendment match implementation. Record all three work orders' evidence
   before promoting the requirement/design and marking the plan implemented.

## ASCII UI preview

Use [UI-01 through UI-03](plan.md#ascii-ui-preview). Keep these excerpts synchronized.

```text
UI-01 desktop: All repositories [OFF]
Organizations [acme x]          [+ Add organization]
Repositories  [outside/tool x]  [+ Add repository]
Shared bar: [Discard] [Save changes]

UI-02 phone: same controls stacked, chips wrap, shared picker behavior
All repositories [OFF]
Organizations [acme x] [+ Add organization]
Repositories [outside/tool x] [+ Add repository]
[Discard] [Save changes]

UI-03: All ON hides pickers. Empty OFF denies all. Save error preserves draft.
```

Entry, page scroll owner, and phone picker surfaces remain the existing GitHub
settings pattern. Reuse shared touch sizing and safe-area behavior. No new
phone layout, dynamic viewport system, or fine-pointer geometry sweep is required.

## Verification

Before UI changes, run the new component test and desktop smoke test named
`saves owner and repository scope together`, and record the expected failures.
After implementation, run this block from repo root:

```bash
(cd apps/web && pnpm exec vitest run components/github/github-repo-scope-section.test.tsx components/github/repo-filter-selector.test.tsx lib/github/repo-scope.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/backend && go run ./cmd/settings-catalog)
node --test scripts/settings-contract-ci.test.mjs
(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-repository-scope-union.spec.ts tests/integrations/github-workspace-settings.spec.ts tests/automations-github-repository-picker.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-github-repository-scope-union.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Install with `(cd apps && pnpm install --frozen-lockfile)` before the first
pnpm command in a fresh worktree. Managed E2E commands rebuild assets. Run
projects sequentially and retain worker guards. Run changed-Go lint with the
explicit affected paths. Rerun prior unit commands only after a relevant change.

## Risks and results

Legacy requests remain exclusive and can clear the other list. Reload old SPA
clients after upgrade. Keep the scope-editor discovery route distinct from
ordinary scope-filtered discovery.

Results:

- The three focused Vitest files pass 28 tests. Web typecheck, Oxlint, generated
  pseudo/i18n validation, and production Vite build pass.
- The desktop GitHub workspace settings and picker E2E set passes 9 tests. The
  mobile repository-scope save/reload E2E passes 1 test.
- Settings catalog generation and its CI contract tests pass.
- Public docs tests and validation, documentation catalog validation, full spec
  lint, and `git diff --check` pass.
- Existing locale keys supply all visible copy; no real locale catalog entries
  were needed.
