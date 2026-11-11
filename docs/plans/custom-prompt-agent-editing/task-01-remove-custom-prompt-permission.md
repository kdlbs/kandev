---
id: "01-remove-custom-prompt-permission"
title: "Remove custom prompt agent permission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
acceptance_criteria:
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.1
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.2
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.3
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.7
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.8
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.9
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.10
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.11
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.12
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.13
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.14
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.15
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.16
system_design:
  - ../../specs/integrations/system-design/shared-prompt-writes.md
---

# Task 01: Remove custom prompt agent permission

## Summary

Deliver the simplified custom-prompt editing policy across persistence, APIs,
configuration instructions, and the existing desktop and phone editor.
The [plan](plan.md) supplies test mapping and delivery boundaries.

## In scope

- Meaningful TDD cases for previously disabled custom prompts, authority, built-ins,
  concurrent edits, stale editor saves, compatibility metadata, and historical schema replay.
- Remove the model permission, service default/permission branch, and SQL column use.
  Fresh schema omits the column. Existing columns remain unused.
- Keep `allow_agent_edits` reads as derived eligibility. Ignore legacy HTTP input,
  including a non-mutating permission-only patch. Reject generic permission changes.
- Remove the editor control, component, form state, payloads, and unused locale keys.
- Include the edit-start prompt revision in editor saves. Refresh pristine open forms
  from remote changes and reject stale dirty saves without overwriting newer content.
- Replace obsolete permission tests with the plan's content-editing and live-refresh scenarios.
- Update configuration instructions, MCP descriptions, and both affected public pages.
- After all checks pass, promote the requirement to `active`, the design to `current`,
  and the plan to `implemented`. Record actual command results in this work order.

## Out of scope

Built-in agent editing, broader tool availability, approval protocols, workflow
rewrites, captured running-turn instructions, unrelated settings changes, and publication.

## Acceptance

- All custom prompts accept existing authorized agent updates, regardless of historical
  permission or creation path. Built-ins, unauthorized requests, and conflicting writes remain rejected.
- Desktop and phone prompt editors save name/content without a permission control
  or payload. Pristine forms adopt remote updates; dirty forms preserve drafts and
  stale saves return a conflict without replacing the current prompt.
- Compatibility reads, historical upgrades, explicit-request instructions, and public
  documentation match the paired design. Every required command has recorded evidence.

## UI preview

UI-01: Settings > Prompts, editing a custom prompt, desktop and phone.
See the [combined preview](plan.md#ui-preview), rendered through Kandev rich output
in the design handoff.

The editor contains prompt name, prompt content, and Cancel, in that order.
The existing floating bar contains Discard and Save changes for a dirty form.
No agent permission row appears. Phone editing retains one page scroll owner,
existing touch targets, safe-area handling, and shared save logic.
This region maps to `.12`, `.9`, and `.7` of the linked requirement.

## Verification

Before package commands in a fresh worktree, install dependencies with
`(cd apps && pnpm install --frozen-lockfile)`.

The PostgreSQL cases require an isolated test database configured through
`KANDEV_TEST_POSTGRES_DSN`. Do not report skipped PostgreSQL cases as successful evidence.
With this variable set, the first backend command includes the PostgreSQL prompt cases.

From the repository root:

```bash
(cd apps/backend && go test -race ./internal/prompts/... ./internal/mcp/handlers ./internal/mcp/server ./config/prompts -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*(Prompt|Settings)' -count=1)
(cd apps/backend && go test ./internal/persistence/storeconformance -run 'Test(UpgradeFixtureManifest|PreviousStableUpgrade)$' -count=1)
git diff --name-only --diff-filter=ACMR -z origin/main...HEAD -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --name-only --diff-filter=ACMR -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --cached --name-only --diff-filter=ACMR -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
(cd apps/web && pnpm exec vitest run components/settings/prompts-settings.test.tsx components/settings/prompts-settings-revisions.test.tsx components/settings/prompts-settings-draft-meta.test.tsx hooks/domains/settings/use-custom-prompts.test.tsx lib/ws/handlers/prompts.test.ts lib/ws/use-websocket.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/prompts-settings.tsx components/settings/use-prompt-editor-effects.ts components/settings/prompts-settings.test.tsx components/settings/prompts-settings-revisions.test.tsx lib/api/domains/settings-api.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/settings/shared-prompt-mcp-writes.spec.ts tests/settings/prompts-settings.spec.ts -- --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-prompts-settings.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The implementation must retain targeted coverage for generic-settings denial
of permission input, agent authority, and both database dialects.
Capture and inspect desktop and phone editor screenshots during the managed E2E runs.
Compare them with UI-01 and record any unresolved difference.
The following documentation-linkage preflight validates coverage references.
It performs no runtime mutation and does not replace the actual PR coverage check.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const workOrder = 'docs/plans/custom-prompt-agent-editing/task-01-remove-custom-prompt-permission.md';
const paths = [
  workOrder,
  'docs/plans/custom-prompt-agent-editing/plan.md',
  'docs/specs/integrations/requirements/shared-prompt-writes.md',
  'docs/specs/integrations/system-design/shared-prompt-writes.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: workOrder, status: 'added', additions: 150, changes: 150 },
    { filename: 'apps/backend/internal/prompts/service/agent_writes.go', status: 'modified' },
  ],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/backend/internal/prompts/models/prompt.go`
- `apps/backend/internal/prompts/dto/dto.go`
- `apps/backend/internal/prompts/controller/controller.go`
- `apps/backend/internal/prompts/service/service.go` and `agent_writes.go`
- `apps/backend/internal/prompts/store/sqlite.go` and `agent_writes.go`
- Existing prompt repository, service, and HTTP handler tests.
- `apps/backend/internal/mcp/handlers/config_prompt_handlers.go` and `config_prompt_writes.go`
- `apps/backend/internal/mcp/server/config_prompt_writes.go` and its tests.
- `apps/backend/internal/backendapp/settings_domain_operations.go` and `settings_prompt_permissions_test.go`
- `apps/backend/config/prompts/config-context.md` and existing context tests.
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json`
- `apps/web/components/settings/prompts-settings.tsx` and its component tests.
- `apps/web/components/settings/prompt-agent-permission.tsx` (remove).
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/src/locales/*/settings.json` (remove the two unused permission keys).
- The three Playwright files named in the plan.
- `docs/public/automation-and-mcp.md` and `docs/public/developer-tools.md`
- The owning requirement/design, decision records, and this plan package.

Retain the optional read property in `apps/web/lib/types/http.ts` for API compatibility.
Do not remove historical locale guard entries or rewrite generated release history.

## Dependencies

None. The implementation followed an explicit user request. The owning artifacts
and scoped backend/web guidance were read before production changes.

## Risks

Historical permission values must never remain a hidden restriction.
SQL simplification must retain built-in and conflict guards.
Old clients must receive truthful effective metadata.
The shared editor must preserve local drafts during remote refresh.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/shared-prompt-writes.md)
- [System design](../../specs/integrations/system-design/shared-prompt-writes.md)
- [Decision](../../decisions/2026-11-11-custom-prompt-agent-editing.md)
- Existing agent-write, generic-settings, upgrade, component, and E2E tests mapped in the plan.

## Results

Implementation and required verification are complete.

- `(cd apps/backend && go test -race ./internal/prompts/... ./internal/mcp/handlers ./internal/mcp/server ./config/prompts -count=1)` with `KANDEV_TEST_POSTGRES_DSN` set to a disposable local PostgreSQL 16 instance: passed, including PostgreSQL prompt-store guards.
- `(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*(Prompt|Settings)' -count=1)` with the same isolated instance: passed.
- `(cd apps/backend && go test ./internal/persistence/storeconformance -run 'Test(UpgradeFixtureManifest|PreviousStableUpgrade)$' -count=1)` with the same isolated instance: passed for SQLite and PostgreSQL.
- Changed-Go lint: passed with 0 issues.
- Focused frontend Vitest: 34 tests passed across 5 files. Web typecheck, `i18n:check`, and focused Oxlint passed.
- The E2E host backend and Vite pseudo-locale build passed. Chromium passed 9 tests; Mobile Chrome passed 2 tests. Both final suites ran with `CAPTURE_PR_ASSETS=1`.
- Captured desktop and phone screenshots were inspected against UI-01. The desktop shows name, content, Cancel, and the floating Discard/Save bar, with no permission row. The phone shows the same editor order and touch-sized floating save actions; its existing bottom bar overlays part of the content editor at the captured scroll position. No structural mismatch remains.
- After promoting specification/package statuses, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py` (36 tests), `python3 scripts/lint-spec-files.py --all`, `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, and `git diff --check` passed.
- Documentation-linkage preflight returned `status: covered` with no errors.

### PR fixup verification (2026-11-11)

- `go test -race ./internal/prompts/... ./internal/mcp/handlers ./internal/mcp/server ./config/prompts -count=1`, `go test -race ./internal/backendapp -run 'Test.*(Prompt|Settings)' -count=1`, the focused `storeconformance` upgrade tests, and both working-tree/base changed-Go lint invocations passed. PostgreSQL-only cases were not exercised in this session because `KANDEV_TEST_POSTGRES_DSN` was absent; the isolated PostgreSQL 16 evidence above predates this fixup.
- `pnpm exec vitest run components/settings/prompts-settings.test.tsx components/settings/prompts-settings-revisions.test.tsx components/settings/prompts-settings-draft-meta.test.tsx hooks/domains/settings/use-custom-prompts.test.tsx lib/ws/handlers/prompts.test.ts lib/ws/use-websocket.test.tsx` passed 34 tests across 6 files. `pnpm run typecheck`, `pnpm run i18n:check`, and focused Oxlint passed.
- The managed Chromium run built the host backend and pseudo-locale Vite assets and passed 9 tests; Mobile Chrome passed 2 tests against those artifacts. Both ran with retries disabled. Desktop and phone captures were inspected and compressed.
- `python3 scripts/list-docs.py validate`, all 36 specification-validator tests, `python3 scripts/lint-spec-files.py --all`, both public-doc validators, and `git diff --check` passed after the fixup edits. The documentation-linkage preflight returned `status: covered` with no errors.
