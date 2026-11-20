---
created: 2026-11-12
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001
system_design:
  - ../../specs/integrations/system-design/github-repository-scope.md
legacy_specs: []
---

# Implementation Plan: GitHub Repository Scope Union

## Overview

Replace the exclusive Mode select with All repositories plus both existing
pickers. Reuse the settings fields and search fanout. Clear inactive legacy
lists before enabling one shared owner-or-repository membership rule.

- Issue: [#5284](https://github.com/kdlbs/kandev/issues/5284), assigned to `carlosflorencio`.
- [Requirement](../../specs/integrations/requirements/github-repository-scope.md):
  `REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001`.
- [System design](../../specs/integrations/system-design/github-repository-scope.md).
- Original investigation base: `9b2b8e98e2b7a510560c65ac572d412fa3394eb0`.
- Rebased implementation base: `ea6af0d500e92a20420729a505c2b95c65707b36`.
- Picker dependency [#5283](https://github.com/kdlbs/kandev/pull/5283) merged at
  `35c5b28cc96e327e88f1b530cf885a0c8eefaba7` and is already present.

Implementation is complete across three sequential work orders. The results
and verification evidence are recorded below.

## Scope and simplicity

The integration system owns the scope rule and its UI. Reuse existing fields,
pickers, save flow, query fanout, and consumer checks. Add no policy framework,
new table, token lifecycle, or unrelated refactor.

Include migration, both settings write paths, existing backend/frontend scope
consumers, a toggle using shared phone behavior, and focused regression tests.
Copy/clone/reset changes are limited to compatibility with canonical `selected`.

Defer new broker enforcement. The inspected `gitHubCredentialProvider` does not
read workspace settings. Reader dependencies and checks during lease admission,
refresh, redemption, and warm-cache reuse create a new access boundary.
They are separate follow-up scope. Draft AC .8 is withdrawn and reserved.
Verify only that App minting retains its existing exact repository restriction.
This package must not claim to deliver the issue's broader broker-enforcement
interpretation. No persistent follow-up task has been created.

## Evidence and approach

The temporary Go overlay confirmed that the current service rejects a mixed
patch and that each exclusive mode ignores the other list. Raw stale entries
remain inactive today. Existing predicate/conflicting-patch tests passed.
The overlay was removed before implementation began.

1. Add canonical `selected`, atomic settings updates, and migration after
   `backfillGitHubWorkspaceOwnership()` and before `seedLegacyWorkspaceConnections()`.
   Cover ordinary API and catalog/agent writes through `updateGitHubSettings`.
2. Update existing predicates, search qualifiers/totals/cache keys, watches,
   automations, mentions, host reset, and installation-removal handling.
3. Replace the select, preserve the scope-editor discovery source, update
   canonical response fixtures, and verify desktop plus one mobile flow.
   Finish with brief public documentation and an ADR 0030 amendment.

The first permanent regression is `TestWorkspaceScopeUnionRoundTrip`:
`selected`, owner `acme`, and exact repository `outside/tool` survive save/read.
Both grants match. `outside/other` remains denied.

## Credential compatibility

| Existing source | Effect of this package | Proof |
| --- | --- | --- |
| PAT/CLI or App backend calls | Existing scope checks use union, with existing identity boundaries | Consumer tests |
| Personal backend reads | Union still intersects automation visibility | Mixed visible/hidden response |
| App named token request | Existing per-repository mint behavior stays unchanged | Existing mint-provider test |

Broker lease enforcement, executor inheritance, and already-delivered bearer
permissions are outside this package.

## ASCII UI preview

Labels and order are structural. Example names and spacing are illustrative.
All final product copy uses localization keys.

```text
UI-01: Desktop, Settings > Workspace > Integrations > GitHub
Before: Mode [Organizations v]  [organization picker] [repo picker disabled]
After:  All repositories [OFF]
        Organizations [acme x]          [+ Add organization]
        Repositories  [outside/tool x]  [+ Add repository]
        Shared settings bar: [Discard] [Save changes]

UI-02: Phone, same settings route and shared picker behavior
All repositories [OFF]
Organizations
[acme x] [+ Add organization]
Repositories
[outside/tool x] [+ Add repository]
[Discard] [Save changes]

UI-03: Existing form states
All ON: hide selection controls, credential permissions still apply.
All OFF, both empty: allow nothing, both add controls remain available.
Failed save: preserve draft and allow retry through shared Save.
```

Phone fields stack and chips wrap within the current page scroll owner.
Reuse #5283's picker containment, accessible controls, and shared save surface.
No new phone composition or general geometry sweep is part of this change.

## Tests and work orders

All criteria below refer to `AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001`.
Test names are proposals unless the existing source already defines them.

| Order | Work order | Criteria and evidence | Status |
| --- | --- | --- | --- |
| 01 | [Scope model and migration](task-01-scope-model-and-migration.md) | .2, .4, .9: save/read, raw migration/replay, legacy/null/empty patches, settings-operations write | done |
| 02 | [Existing scope consumers](task-02-existing-scope-consumers.md) | .5-.7, .10, .11: queries, watches, automations, mentions, copy/reset/removal, exact App mint | done |
| 03 | [Settings, browser proof, and docs](task-03-settings-browser-and-docs.md) | .1-.3, .9, .11, .12: frontend predicates, desktop and one mobile spec | done |

PostgreSQL migration/write coverage runs once in work order 01 against an
isolated database. A skipped environment-gated test is not passing evidence.
Each work order contains exact commands. There is no additional generic QA or
verification work order.

## Compatibility and entry risks

- Canonical GET now returns `selected` instead of `orgs`/`repos`. Audit every
  old-mode reader and update canonical fixtures. Preserve dedicated legacy-request tests.
- The actual old GET assertions are in `integrations/github-workspace-settings.spec.ts`.
  Audit the API helper, merged-trigger spec, and remote-task-picker spec too.
  They have no `repo_scope_mode` literals at this base. Avoid unnecessary edits.
- Keep `repositorySource="scope-editor"` and the unfiltered scope-editor
  repository endpoint. The scope organization picker must also remain unfiltered.
  Ordinary repository discovery stays scope-filtered.
- Inactive legacy lists must be cleared using their old mode. Reinterpreting
  old modes directly as union silently grants access.
- Older binaries interpret unknown `selected` as `all`. Binary-only downgrade
  is unsafe. Rollback needs its matching database backup. Mixed-version clients
  are unsupported. A cached SPA's legacy `orgs` save clears exact repositories.
  Reload clients after upgrade.
- Clone's raw column copy is safe because startup migration completes first.
  New clone infrastructure is unnecessary. Removing the final selected entry
  leaves deny-all. Host reset must change both hardcoded mode literals.

## Verification results

Implementation completed on 2026-11-17:

- Full backend `make -C apps/backend test`: passed.
- `make -C apps/backend build`: passed for the backend, agentctl, and helper binaries.
- Changed-package Go lint: passed with 0 issues; changed Go files are gofmt-clean.
- Focused backend race tests passed for GitHub scope/search/watch/migration,
  settings operations, automation, and mentions. The environment-gated
  `TestPostgresWorkspaceScopeUnion` passed against a disposable PostgreSQL 16 database.
- Web component and predicate tests: 28 passed across 3 files. Web typecheck,
  Oxlint, i18n checks, and Vite production build passed.
- Desktop GitHub repository-scope E2E: 9 tests passed. Mobile scope E2E: 1 test passed.
- Settings catalog generation and `node --test scripts/settings-contract-ci.test.mjs` passed.
- Public documentation tests and validation, documentation catalog validation,
  full specification lint, and `git diff --check` passed.
- The pseudo locale was regenerated and passes `i18n:check`; the UI reuses existing
  localized labels, so no real locale catalog entries were added.

The migration skips only legacy workspace-settings tables that do not contain
any repository-scope columns. The full-suite run also required wiring the
existing workflow step service in the Improve Kandev prompt-upgrade test fixture;
that change is test-only.
