---
id: "02-cover-desktop-and-phone"
title: "Prove setup attachment and subsequent chat on desktop and phone"
status: done
wave: 2
depends_on:
  - "01-bind-worktree-script-turns"
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-source-script-turns.md
---

# Task 02: Prove setup attachment and subsequent chat on desktop and phone

## Summary

Prove that the existing Files attachment flow finishes when the added repository has a setup script.
Verify script execution and the next conversational response on desktop and phone.

## In scope

- Add focused `chromium` and `mobile-chrome` scenarios through the current dialog and drawer.
- Use the existing API client, local Git fixtures, causal waits, and session page helpers.
- Cover attachment response, actual script execution, Files content, reload, and subsequent chat.
- Retain existing attachment and initial setup-script regressions as compatibility checks.

## Out of scope

Production UI, layout, copy, translation catalogs, new public APIs, provider-specific behavior, and live remote-executor coverage.
The backend correction belongs to Task 01.

## Acceptance

1. Both scenarios submit through Files and receive attachment success within the existing request budget.
2. The added worktree contains a script-created marker. Its setup message contains actual output, reports success, and survives reload.
3. A later prompt produces a new completed conversational turn and response in the same session. No phantom active turn blocks subsequent attachment.

## Scenario

1. Create an offline local Git repository under the worker's disposable directory.
2. Register that repository with `apiClient.createRepository` and set its script with `apiClient.updateRepository`.
3. Configure a script that creates a marker and emits a distinctive output line.
4. Start a separate primary Worktree task with the existing mock agent.
5. Wait for its environment, `WAITING_FOR_INPUT` state, and completed initial turn through backend reads.
6. Open Files, then **+**, then **Add repositories or folders**.
7. Select the saved script-enabled repository and the existing **Inside the current** placement.
8. Arm the HTTP and relevant WebSocket waits before submission.
9. Submit through the visible dialog or phone drawer.
10. Verify response success, source projection, Files marker contents, script output, and final message status.
11. Reload and verify the same session and persisted script result.
12. Send `/e2e:simple-message` through the composer and verify its response and completed new turn.
13. Reopen attachment and verify that idle eligibility remains available.

Correlate messages by task/session and `metadata.script_type: setup`.
Do not count agent boot cards as repository setup messages.
The marker proves execution even if the command text appears before any output.

Use the existing desktop dialog and phone drawer without changing their geometry.
The phone scenario uses `.tap()` and the existing Files navigation.
`mobile-add-workspace-sources.spec.ts` supplies the nearest shipped interaction pattern.
These are coverage additions without a rendered UI change, so no new composition preview is required.

## Verification

Commands start from the repository root and run sequentially.
Install dependencies once in this worktree before the first package command.
The managed runner rebuilds the backend, mock agent, and web assets. Do not reuse stale binaries or overlap E2E runs.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/workspace-source-setup-script.spec.ts tests/task/add-workspace-sources.spec.ts tests/session/setup-script-no-duplicate.spec.ts tests/session/setup-script-progress.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-workspace-source-setup-script.spec.ts tests/task/mobile-add-workspace-sources.spec.ts -- --retries=0)
(cd apps/web && pnpm exec eslint e2e/tests/task/workspace-source-setup-script.spec.ts e2e/tests/task/mobile-workspace-source-setup-script.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workspace-source-script-deadlock
```

Run this delivery preflight from the repository root. It uses the repository validator with the runtime path and this package:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/workspace-source-script-deadlock';
const orders = fs.readdirSync(dir)
  .filter(name => /^task-\d{2}-.*\.md$/.test(name))
  .map(name => `${dir}/${name}`);
const files = [
  `${dir}/plan.md`, ...orders,
  'docs/specs/tasks/requirements/attach-workspace-sources.md',
  'docs/specs/tasks/system-design/attach-workspace-sources.md',
  'docs/specs/tasks/system-design/workspace-source-script-turns.md',
];
const fileContents = Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')]));
const changedFiles = orders.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/backendapp/worktree.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

Record the accepted work-order and design references.
Verify that the new desktop and phone tests are discovered in their intended projects.
Record actual test counts and any unavailable environment dependency.

Task 01 owns the red deadlock evidence. These browser tests verify the full user path after that correction.
Do not remove the production correction to force an additional browser deadlock run.

## Files likely touched

- `apps/web/e2e/tests/task/workspace-source-setup-script.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-workspace-source-setup-script.spec.ts` (new)
- This work order and `plan.md` for execution results.

Reuse existing helpers without adding production hooks. Keep any scenario-local fixture setup inside these focused specs.

## Dependencies

Task 01 must pass its backend acceptance conditions first.

## Risks

- A task with an empty `task_dir_name` defers materialization and cannot reproduce the live setup path.
- Agent output can precede `WAITING_FOR_INPUT`. UI idle appearance alone is insufficient readiness evidence.
- Generic message text can match command metadata. Marker contents and final setup status must prove actual execution.
- The phone project discovers only `mobile-*.spec.ts` files.

## Parallelism

`sequential`

## Inputs

- [Attachment requirement](../../specs/tasks/requirements/attach-workspace-sources.md), AC-001.2 and AC-001.5 with the full prefix from frontmatter.
- [Attachment design](../../specs/tasks/system-design/attach-workspace-sources.md).
- [Script-turn design](../../specs/tasks/system-design/workspace-source-script-turns.md).
- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`.
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`.
- `apps/web/e2e/tests/session/setup-script-no-duplicate.spec.ts`.
- `apps/web/e2e/tests/session/setup-script-progress.spec.ts`.
- The `/e2e` and `/mobile-parity` skills and scoped web guidance.

## Results

Completed 2026-11-08. Both browser scenarios passed.

Added:

- `apps/web/e2e/tests/task/workspace-source-setup-script.spec.ts` (`chromium`).
- `apps/web/e2e/tests/task/mobile-workspace-source-setup-script.spec.ts` (`mobile-chrome`).

Each scenario creates an offline local Git repository with a bare origin,
registers it with `apiClient.createRepository`, and assigns a setup script that
writes and prints `issue5151-setup-ok`. After the initial task turn completes,
the scenario submits the saved repository through the existing Files dialog or
drawer with the `current_root` placement. It asserts the HTTP response and
`session.workspace_sources.updated`, then verifies the setup message and output,
its completed lifecycle turn, the worktree marker file, the Files tree node, and
its content. It reloads, confirms the persisted source and script result, sends
`/e2e:simple-message`, and requires a new completed turn with the mock agent
response. It then reopens the attachment menu and confirms the action is still
enabled for an idle session.

Commands (from the repository root; `E2E_PORT_OFFSET=21` kept this run on an
isolated port slot while other worktree E2E runs shared the host):

```bash
(cd apps/web && E2E_PORT_OFFSET=21 pnpm e2e:run --host --no-build --project chromium tests/task/workspace-source-setup-script.spec.ts tests/task/add-workspace-sources.spec.ts tests/session/setup-script-no-duplicate.spec.ts tests/session/setup-script-progress.spec.ts -- --retries=0)
(cd apps/web && E2E_PORT_OFFSET=21 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-workspace-source-setup-script.spec.ts tests/task/mobile-add-workspace-sources.spec.ts -- --retries=0)
(cd apps/web && pnpm exec eslint e2e/tests/task/workspace-source-setup-script.spec.ts e2e/tests/task/mobile-workspace-source-setup-script.spec.ts)
```

Results:

- Desktop suite: 11 tests passed in 1.8 minutes, including both new tests and
  the existing attachment and setup-script regressions.
- Phone suite: 2 tests passed in 27.5 seconds, including the new mobile test and
  the existing `mobile-add-workspace-sources` regression.
- Focused ESLint: passed with no findings.
- `python3 scripts/list-docs.py validate`: 437 decisions and 1938 specifications
  validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- Delivery preflight through `.github/scripts/pr-docs.cjs` returned
  `status: covered` with no errors and accepted both pending work orders against
  both system designs.
