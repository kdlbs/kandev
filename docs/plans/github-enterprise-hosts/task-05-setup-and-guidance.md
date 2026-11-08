---
id: "05-setup-and-guidance"
title: "Setup surfaces and public guidance"
status: complete
wave: 5
depends_on: ["04-task-git-access"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-005
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.4
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.4
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 05: Setup surfaces and public guidance

## Summary

Finish user-visible Enterprise setup and recovery with the existing CLI picker and host-aware PAT/App forms. Update frontend identities/links, desktop/phone proof, localized copy, and public setup guidance.

## In scope

- Keep CLI account selection authoritative for host; add host input only where needed for PAT/App setup and filter App catalog/options by that host.
- Share responsive drafts, cancel/ignore stale host/workspace requests, display selected host and replacement effects, and expose useful errors.
- Propagate host through frontend URL helpers, repository import and branch hooks, association keys, API types, and mocks.
- Add all seven locale values and desktop/mobile E2E for the exact reported regression plus PAT/App/OAuth and host-replacement behavior.
- Update `docs/public/integrations.md` as a how-to guide and reconcile any applicable README/screenshot descriptions; promote the draft pair only after all five orders pass.

## Out of scope

No second CLI host picker, generic settings redesign, new integration framework, or real corporate secrets in fixtures.

## Acceptance

1. Existing CLI dropdown selection saves/reloads with the Enterprise host visible; PAT/App setup produces correct hosts/links, and failed/stale requests cannot mislabel the active connection.
2. Desktop dialog and phone Drawer match UI-01/UI-02, support complete setup/recovery, retain drafts across resize, and pass touch/keyboard/overflow checks with all required copy translated.
3. Frontend links and caches preserve host identity; documentation explains version/trust/network and host-change limits without claiming unavailable live-provider evidence.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview). These excerpts retain
UI-01/UI-02 grouping and state semantics; spacing/copy is illustrative.

UI-01, desktop CLI connection (AC-001.1, AC-005.1, AC-005.2):

```text
Change GitHub connection
Method: PAT | [GitHub CLI] | App
Account [alice (git.company.example) v]
Host: git.company.example
Task Git access [Inherit executor v]
                         [Save changes]
```

UI-02, phone PAT/App connection (AC-005.1 through AC-005.4):

```text
+----------------------------------+
| Change GitHub connection    [X] | fixed
|----------------------------------|
| Method [PAT v]                   |
| Host [git.company.example      ] |
| Token [************************] | scroll
| Task Git access [Managed v]      |
| Host-change effects / errors     |
|----------------------------------|
| [        Save changes          ] | fixed + safe area
+----------------------------------+
```

CLI mode keeps the host-labelled account picker; no extra hostname entry.
App mode replaces token with App selection/create/import/install and existing
callback guidance. App installation keeps its own explicit action and completion
flow. Empty, loading, invalid-host, failed-auth, TLS, and host-replacement states
follow the plan. Use the existing responsive connection dialog/Drawer and
shared draft, one scroll owner, >=44px phone targets, and 28px desktop controls.

## Verification

Start at repository root. Read scoped AGENTS.md and use `/tdd` for changed
logic. New worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Backend changes also run `bash scripts/lint-go-changed` with the explicit changed
Go file paths; frontend changes run ESLint on the explicit changed TS/TSX paths.
Record the expanded commands in Results. Do not mark skipped database tests as passed.

```bash
(cd apps/web && pnpm exec vitest run components/github/github-connection-settings-form.test.tsx components/github/github-connection-dialog.test.tsx components/github/github-app-onboarding-model.test.ts lib/github/parse-url.test.ts lib/utils/github-repo-url.test.ts lib/utils/external-vcs-file-url.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-enterprise-hosts.spec.ts tests/integrations/github-authentication.spec.ts tests/settings/github-app-registration.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-github-enterprise-hosts.spec.ts tests/integrations/mobile-github-auth-settings.spec.ts tests/settings/mobile-github-app-registration.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Generate Traditional Chinese with `pnpm run i18n:zh-hant` from
`apps/web` before checking catalogs. Include every additional changed hook/API
suite in the final Vitest invocation. Managed E2E rebuilds production assets;
retain screenshots and compare structure to the assigned previews.

## Files likely touched

- `apps/web/components/github/github-connection-dialog.tsx`, `github-connection-settings-form.tsx`, `github-cli-form.tsx`, `github-pat-form.tsx`, App forms/guides and status summary.
- `apps/web/lib/api/domains/github-api.ts`, `github-auth-api.ts`, GitHub types, URL utilities, remote-repository/branch hooks, PR/issue association hooks and their tests.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/github.json` and other affected namespaces.
- `apps/web/e2e/tests/integrations/github-enterprise-hosts.spec.ts`, `mobile-github-enterprise-hosts.spec.ts` (new), existing settings specs/page object, API helper and `apps/backend/internal/github/mock_controller.go`.
- `docs/public/integrations.md`, affected authentication spec scope notes, and this package's requirements/design/status/results.

## Dependencies

04-task-git-access. Follow the plan's acceptance order and authorized implementation lanes.

## Risks

Mock E2E is UI evidence; task 02 proves the backend guard. App callbacks require reachable Kandev URLs, not necessarily public-internet URLs for private GHES. Run desktop/mobile managed suites sequentially. Record live provider smoke separately if credentials/environments are supplied.

## Parallelism

Parallel-safe only within the explicitly owned implementation lane in plan.md; integration acceptance remains dependency-ordered.

## Inputs

- [Requirements](../../specs/integrations/requirements/github-enterprise-hosts.md), using the exact IDs above.
- [System design](../../specs/integrations/system-design/github-enterprise-hosts.md) and [host/OAuth ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md).
- Existing source and neighboring tests named in Files likely touched; the plan's test matrix defines new regression names.

## Results

Implementation is complete. The existing CLI dropdown remains the selection
control; PAT/App setup now carries an explicit host. Draft host, App catalog,
resource URLs, historical PR actions and workflow-sync configuration retain their
origin. All supported locale catalogs include the setup copy.

The final frontend matrix passes: 584 tests in 44 suites, TypeScript typecheck
with `NODE_OPTIONS=--max-old-space-size=6144`, changed-source ESLint with zero
errors/warnings, and both i18n checks. Coverage includes setup, request/response
races, resource keys/URLs, historical PR actions, settings/presets, workflow
sync, and App secret clearing. Managed browser checks passed after fresh
backend/helper/web builds: 12 desktop and 8 mobile cases, including Enterprise
installation and personal OAuth. A final CLI field-order adjustment also passed
15 focused tests and lint. Only completed browser runs count as evidence.

Public docs updated: `docs/public/integrations.md` and
`docs/public/workflow-sync.md` (how-to guides with field references). Existing
authentication requirement/design contracts now reference the host and OAuth
extension. Public documentation tests (63 tests), all 48 published-page validation,
specification catalog validation and full specification lint pass.

Expanded frontend validation commands (from repository root):

```bash
(cd apps/web && pnpm exec vitest run components/github/github-app-import-form.test.tsx components/github/github-app-onboarding-model.test.ts components/github/github-connection-dialog.test.tsx components/github/github-connection-draft.test.tsx components/github/github-status.test.tsx components/github/my-github/pr-list.test.tsx components/github/my-github/quick-task-launcher.test.tsx components/github/my-github/use-default-query-presets.test.ts components/github/my-github/use-pr-statuses.test.ts components/github/my-github/use-saved-presets-concurrency.test.ts components/github/my-github/use-saved-presets.test.ts components/github/pr-detail-panel-mapping.test.ts components/github/pr-detail-panel.test.ts components/github/pr-detail-panel.test.tsx components/github/pr-merge-button.test.tsx components/settings/workflow-sync-dialog.test.tsx components/task-create-dialog-effects.test.ts components/task-create-dialog-helpers.test.ts components/task/use-review-dialog.test.ts hooks/domains/github/pr-feedback-resource.test.ts hooks/domains/github/use-branches-by-url.registry.test.ts hooks/domains/github/use-branches-by-url.test.ts hooks/domains/github/use-issue-key-to-tasks.test.ts hooks/domains/github/use-pr-ci-popover.test.ts hooks/domains/github/use-pr-commits.test.ts hooks/domains/github/use-pr-diff.test.ts hooks/domains/github/use-pr-feedback.test.ts hooks/domains/github/use-pr-info-by-url.registry.test.ts hooks/domains/github/use-pr-info-by-url.test.ts hooks/domains/github/use-pr-key-to-tasks.test.ts hooks/domains/github/use-repo-merge-methods.test.ts hooks/domains/integrations/use-remote-repositories.test.tsx hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/session/use-review-sources.test.ts hooks/domains/settings/use-workflow-sync.lifetime.test.tsx hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx hooks/domains/settings/use-workflow-sync.test.ts lib/api/domains/github-api.test.ts lib/api/domains/github-pr-api.test.ts lib/api/domains/github-review-api.test.ts lib/github/host.test.ts lib/github/parse-url.test.ts lib/utils/external-vcs-file-url.test.ts lib/utils/github-repo-url.test.ts)
(cd apps/web && pnpm exec eslint components/github/github-app-connection-panel.tsx components/github/github-app-create-form.tsx components/github/github-app-import-form.tsx components/github/github-app-onboarding-model.ts components/github/github-connection-dialog.tsx components/github/github-connection-draft.tsx components/github/github-connection-settings-form.tsx components/github/github-pat-form.tsx components/github/github-repo-scope-section.tsx components/github/github-status.tsx components/github/my-github/issue-list.tsx components/github/my-github/pr-list.tsx components/github/my-github/quick-task-launcher.tsx components/github/my-github/use-default-query-presets.ts components/github/my-github/use-pr-statuses.ts components/github/my-github/use-saved-presets.ts components/github/pr-detail-panel.tsx components/github/pr-merge-button.tsx components/github/use-pr-scoped-review-request.ts components/settings/workflow-sync-dialog.tsx components/task-create-dialog-effects.ts components/task-create-dialog-helpers.ts components/task/new-subtask-dialog.tsx components/task/use-review-dialog.ts hooks/domains/github/pr-commits-resource.ts hooks/domains/github/pr-feedback-resource.ts hooks/domains/github/use-branches-by-url.ts hooks/domains/github/use-issue-key-to-tasks.ts hooks/domains/github/use-pr-ci-popover.ts hooks/domains/github/use-pr-commits.ts hooks/domains/github/use-pr-diff.ts hooks/domains/github/use-pr-feedback.ts hooks/domains/github/use-pr-info-by-url.ts hooks/domains/github/use-pr-key-to-tasks.ts hooks/domains/github/use-repo-merge-methods.ts hooks/domains/integrations/use-remote-repositories.ts hooks/domains/session/use-remote-contribution-relation.ts hooks/domains/session/use-review-sources.ts hooks/domains/settings/use-workflow-sync.ts lib/api/domains/github-api.ts lib/api/domains/github-pr-api.ts lib/api/domains/github-review-api.ts lib/github/host.ts lib/github/parse-url.ts lib/types/github-app.ts lib/types/github.ts lib/types/workflow-sync.ts lib/utils/external-vcs-file-url.ts lib/utils/github-repo-url.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
```

Final browser commands (repository root, one worker per managed shard):

```bash
PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=1 pnpm --dir apps/web e2e:run --project chromium e2e/tests/integrations/github-enterprise-hosts.spec.ts e2e/tests/integrations/github-authentication.spec.ts e2e/tests/settings/github-app-registration.spec.ts --grep-invert 'keeps App installation'
PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=1 pnpm --dir apps/web e2e:run --project chromium e2e/tests/integrations/github-enterprise-hosts.spec.ts --grep 'keeps App'
PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=1 pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/integrations/mobile-github-enterprise-hosts.spec.ts e2e/tests/integrations/mobile-github-auth-settings.spec.ts e2e/tests/settings/mobile-github-app-registration.spec.ts
```

Results respectively: 11 passed (29.9s), 1 passed (15.5s), 8 passed (31.0s).
Phone checks cover 44px targets, one internal scroll body, no horizontal
overflow, draft preservation across resizing, and installation/OAuth callbacks.

Rendered evidence was visually inspected against UI-01/UI-02: desktop method,
account, host and task-access grouping; phone fixed header/footer, stacked
controls, one scroll body, reachable lower fields and no horizontal clipping.
The final phone capture test passed after waiting for the Drawer to enter the
viewport (1 passed, 10.4s). Local ignored artifacts are retained at:

- `apps/web/e2e-artifacts/github-enterprise-hosts/enterprise-cli-setup-desktop.png`
- `apps/web/e2e-artifacts/github-enterprise-hosts/enterprise-pat-setup-mobile.png`

These are local review artifacts, not committed public-document screenshots.

PR fixup: merged the current base's reactive subtask parent defaults while retaining
workspace-specific GitHub URL validation. Updated the subtask form-state test
harness to supply the real state provider required by host-aware lookups. No
product contract changed. Verification: 13 related Vitest files (127 tests),
frontend typecheck and focused ESLint passed. Managed subtask E2E passed 3 cases
covering sidebar creation, pasted GitHub URLs, and selected-session inheritance.

CI remediation updates existing PR-refresh assertions to include the host, and
separates workflow-sync config requests from GitHub status reads in the transport
fixture. The three failing frontend files reproduced all nine CI assertions;
the repaired scope and two adjacent lifetime suites pass (5 files, 82 tests).
Backend status fixtures now use deterministic provider responses instead of
contacting GitHub with fixture credentials. The issue-link cancellation test
observes admission at the final write boundary after host/repository validation.
These are test-only adjustments to the existing contracts; no UI recapture or
requirement change is needed. Remote CI remains pending for the next push.

The complete GitHub package passed under `go test -race -trimpath -p 1
./internal/github -count=1` in 173.861 seconds. Temporary HTTP tracing recorded
zero unmocked provider requests; the diagnostic file was removed afterward.

The ambient CI job also exposed a cancellation-test timing race: its next
credential read overlapped the detached rotation commit and correctly rejected
the stale snapshot. The fixture now signals the committed rotation before that
read, retaining the fail-closed credential binding. PostgreSQL CI reported the
same provider-fixture leaks and issue-link cancellation assertion as SQLite.
Focused frontend ESLint and the targeted GitHub/backendapp race tests passed.
The complete GitHub race suite also passed with the CI ambient integration
environment (173.161 seconds), and the issue-link cancellation boundary passed
against PostgreSQL under the race detector (5.362 seconds).


### Greptile follow-up

Status refreshes no longer replace the selected method in an open connection draft. Four deferred-save cases cover success and failure on desktop and phone. The dialog suite passes all 17 cases, including stale dismissal/reopen behavior. This changes shared state ownership only, with no visual or touch changes.
