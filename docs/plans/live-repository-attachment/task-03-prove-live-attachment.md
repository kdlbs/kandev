---
id: "03-prove-live-attachment"
title: "Prove live attachment on desktop and phone"
status: in_progress
wave: 3
depends_on:
  - "01-preserve-chat-file-context"
  - "02-reconcile-git-refresh-scopes"
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.8
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.41
system_design:
  - ../../specs/tasks/system-design/chat-file-link-context.md
  - ../../specs/platform/system-design/git-repository-scope-lifecycle.md
---

# Task 03: Prove live attachment on desktop and phone

## Summary

Add one reusable mock-agent scenario and focused desktop/phone browser coverage for the real live attachment path.
Complete browser acceptance for Tasks 01 and 02 without changing production attachment semantics.

## In scope

- Add `/e2e:live-repository-attachment` to the mock-agent scenario registry.
- Use the scenario's real session MCP client to call `add_branch_to_task_kandev` during the active turn.
- Emit a primary relative link before attachment and relative, absolute, and explicit source links afterward.
- Use disposable repositories with distinct content at duplicate filenames and existing primary changes.
- Keep Changes mounted, then assert named snapshots, preserved files, and absence of the obsolete warning.
- Repeat links and status assertions after reconnect/reload. Include a parent-root session.
- Record specification, documentation-coverage, and diff gates for the complete package.

## Out of scope

Testing another provider's sandbox, substituting idle batch attachment, broad E2E suites, and adding a test-only production bypass.

## Acceptance

1. The scenario makes a real legacy tool call with a second repository registered in the same disposable workspace.
   Assert tool success, unchanged native session/CWD, and distinct effective root and primary checkout.
2. Chromium and `mobile-chrome` open exact primary and sibling contents through the existing file viewers.
   Old and new relative links survive promotion and reload. A parent-root session keeps its parent-relative meaning.
3. Real named repository status reaches Changes and clears the obsolete warning while primary changes remain visible.
   Reconnect does not restore it. All named implementation and documentation commands have recorded results.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).

```text
Desktop: Chat -> File panel       Changes -> named repository groups
Phone:   Chat -> full-height file viewer -> Back
         Changes tab -> same current repository state
```

Compare the rendered phone result with the fixed viewer controls, internal scroller, and safe-area notes.
Preserve desktop layout preferences and existing mobile navigation.

## Verification

Run from the repository root. Each managed browser command builds current binaries and web assets.
Run the two browser commands sequentially with the runner's normal worker budget.

```bash
(cd apps/backend && go test -tags fts5 ./cmd/mock-agent -run 'TestLiveRepositoryAttachment')
bash scripts/lint-go-changed apps/backend/cmd/mock-agent/scenarios.go apps/backend/cmd/mock-agent/live_repository_attachment.go apps/backend/cmd/mock-agent/live_repository_attachment_test.go
(cd apps/web && pnpm exec oxlint -c oxlint.e2e-sleeps.config.mjs --deny-warnings e2e/helpers/live-repository-attachment.ts e2e/tests/task/live-repository-attachment.spec.ts e2e/tests/task/mobile-live-repository-attachment.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/live-repository-attachment.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-live-repository-attachment.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/live-repository-attachment
```

Record the actual discovered test count, scenario results, phone rendering, and cleanup.
Run this local documentation-coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const pkg = 'docs/plans/live-repository-attachment';
const paths = [
  `${pkg}/plan.md`,
  ...fs.readdirSync(pkg).filter(name => /^task-\d{2}-.*\.md$/.test(name)).map(name => `${pkg}/${name}`),
  'docs/specs/tasks/requirements/attach-workspace-sources.md',
  'docs/specs/platform/requirements/workspace-git-status.md',
  'docs/specs/tasks/system-design/chat-file-link-context.md',
  'docs/specs/platform/system-design/git-repository-scope-lifecycle.md',
];
const changedFiles = execFileSync('git', ['ls-files', '--modified', '--others', '--exclude-standard'], { encoding: 'utf8' })
  .trim().split('\n').filter(Boolean).map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({
  changedFiles,
  fileContents: Object.fromEntries(paths.map(name => [name, fs.readFileSync(name, 'utf8')])),
});
process.stdout.write(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2) + '\n');
process.exitCode = result.ok ? 0 : 1;
NODE
```

This command includes untracked work orders without staging or committing them.

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`.
- New `apps/backend/cmd/mock-agent/live_repository_attachment.go` and `live_repository_attachment_test.go`.
- New `apps/web/e2e/helpers/live-repository-attachment.ts`.
- New `apps/web/e2e/tests/task/live-repository-attachment.spec.ts`.
- New `apps/web/e2e/tests/task/mobile-live-repository-attachment.spec.ts`.
- This plan, its work orders, and the paired draft designs for final delivery status.

Reuse `mcp_client.go`, existing add-sources fixtures, `SessionPage`, and causal waits.
Keep the scenario arguments scoped to the current mock session and registered workspace repository.

## Dependencies

Tasks 01 and 02. Add the scenario before browser proof, then run both viewports on fresh managed builds.

## Risks

An idle attachment or synthetic store update cannot establish this active-turn regression.
Do not use fixed sleeps to wait for materialization or named snapshots.
No claim of browser success is valid without a completed run and exact file-content assertions.

## Parallelism

`sequential`

## Inputs

Read both linked designs and the plan's E2E matrix.
Follow `/e2e` and `/mobile-parity` with the configured fixtures and phone project.

## Results

Implemented the arg-aware mock-agent scenario, the shared browser helper, and
the desktop and phone specs. The scenario calls the real
`add_branch_to_task_kandev` tool during the active turn and emits links before
and after the call.

Recorded evidence:

- `(cd apps/backend && go test -tags fts5 ./cmd/mock-agent -run 'TestLiveRepositoryAttachment')`:
  passed.
- `(cd apps/web && pnpm e2e:run --host --project chromium tests/task/live-repository-attachment.spec.ts)`:
  1 passed (14.8 s).
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-live-repository-attachment.spec.ts)`:
  1 passed (10.1 s).
- Both flows preserved the initial checkout for relative links, opened exact
  primary and sibling contents, asserted the secondary repository's changed
  file, retained both Changes groups, and showed no obsolete refresh warning
  after attachment and reload.
- Duplicate filenames and explicit source prefixes are browser-covered in both
  viewports. Parent-root relative semantics remain covered by resolver,
  provider, and fallback-store regressions; no parent-root browser session is
  claimed.
- The phone proof uses `mobile-chrome` emulation. A physical-device smoke check
  of the mobile Chat and Changes flows was unavailable here and remains a
  release gate before closing the issues.

The plan and both paired designs are promoted to their final status. The ADR
is accepted.
