---
id: "01-repair-repository-scope"
title: "Repair question image repository scope"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-002.1
  - AC-TASKS-QUESTION-IMAGES-002.3
  - AC-TASKS-QUESTION-IMAGES-002.4
  - AC-TASKS-QUESTION-IMAGES-002.6
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.2
  - AC-TASKS-QUESTION-IMAGES-004.5
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 01: Repair question image repository scope

## Summary

Add the guarded own-folder alias to the existing image reader. Clarify the
shared tool schema and public guide. Keep the implementation to this conditional
and text correction.

## In scope

- The scope conditional and focused reader, HTTP endpoint, and schema regressions.
- A short correction to the public image-question guide.
- Existing backend and desktop/phone compatibility checks.

## Out of scope

New resolver layers, caches, runtime fields, routes, preflight reads, tracker
changes, frontend changes, and new browser fixtures.

## Acceptance

1. The own-name regression fails with `not_found` before the fix and passes afterward.
2. Existing same-name children retain precedence. Failed child reads never select root files.
3. Both schema placements and the guide explain omission and child scopes.
   Focused regressions and existing compatibility checks pass.

## Implementation

After an explicit implementation request, mark the work order and plan
`in_progress`. Add the reader and HTTP regressions first and run them red.
Use existing `initGitRepoAt`, `writeQuestionImageFixture`, and API
`initWorkspaceGitRepo` helpers.

Implement the three conditions in the design's
[scope-selection section](../../specs/tasks/system-design/question-image-previews.md#repository-scope-selection).
Keep the conditional inside `openQuestionImageRoot`. Add the schema regression
and guidance correction. Run the commands below, record actual results, and
synchronize package status.

## Verification

Run commands from the repository root. Before the correction, run:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^TestQuestionImageSelfRepositoryScope$' -v -count=1 -timeout=3m)
```

After the correction, run these commands sequentially:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process -run '^TestQuestionImage' -count=1 -timeout=3m)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run '^TestQuestionImage' -count=1 -timeout=3m)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run '^Test(QuestionImages|ChoiceImages)' -count=1 -timeout=3m)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/question_images.go apps/backend/internal/agentctl/server/process/question_images_repo_scope_test.go apps/backend/internal/agentctl/server/api/question_images_test.go apps/backend/internal/mcp/server/question_images.go apps/backend/internal/mcp/server/ask_user_question_images_test.go
(cd apps && pnpm install --frozen-lockfile)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/question-image-previews.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/question-image-repository-scope
```

Install workspace dependencies before the first pnpm command. The phone run
intentionally reuses the fresh desktop build without source changes.

Run this local documentation preflight during planning and implementation.
The projected runtime path forces full traceability validation during planning.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const prefix = 'docs/plans/question-image-repository-scope/';
const documents = [
  prefix + 'plan.md',
  prefix + 'task-01-repair-repository-scope.md',
  'docs/specs/tasks/requirements/question-image-previews.md',
  'docs/specs/tasks/system-design/question-image-previews.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/agentctl/server/process/question_images.go', status: 'modified' },
    { filename: prefix + 'task-01-repair-repository-scope.md', status: 'added' },
  ],
  fileContents: Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/question_images.go`
- `apps/backend/internal/agentctl/server/process/question_images_repo_scope_test.go` (new)
- `apps/backend/internal/agentctl/server/api/question_images_test.go`
- `apps/backend/internal/mcp/server/question_images.go`
- `apps/backend/internal/mcp/server/ask_user_question_images_test.go`
- `docs/public/agent-communication.md`
- This work order and `plan.md` for actual results.

## Dependencies

None.

## Risks

An unconditional basename rewrite can hide a child repository. Keep the
absent-child guard and reuse existing file-containment checks.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/question-image-previews.md), requirements 002 and 004.
- [System design](../../specs/tasks/system-design/question-image-previews.md#repository-scope-selection).
- [Root-cause evidence and focused test matrix](plan.md).
- Existing process image tests and `TestQuestionImageEndpoint`.
- Existing MCP image schema tests and desktop/phone image-question specs.
- `.agents/skills/tdd/references/backend-tests.md` and `.agents/skills/e2e/SKILL.md`.

## Results

Done. The permanent self-repository regression failed before the fix with
`not_found`, while the omitted-scope control passed. Reader precedence, HTTP,
and MCP schema regressions pass.

Verification passed:

- Focused process, API, and MCP Go tests from this work order.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/question_images.go apps/backend/internal/agentctl/server/process/question_images_repo_scope_test.go apps/backend/internal/agentctl/server/api/question_images_test.go apps/backend/internal/mcp/server/question_images.go apps/backend/internal/mcp/server/ask_user_question_images_test.go`: 0 issues.
- `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/question-image-previews.spec.ts --retries=0`: backend and Vite assets built; 4 passed.
- `pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts --retries=0`: 3 passed.
- Public docs tests: 63 passed; public docs validator checked 55 pages.
- Specification catalog validated 452 decisions and 2014 specifications.
- All 36 specification-linter tests and the full specification lint passed.
- Documentation coverage preflight returned `covered` with no errors.
- `git diff --check` passed.

The shared MCP schema and `docs/public/agent-communication.md` now explain
workspace-root and child-repository references, including the guarded legacy alias.
