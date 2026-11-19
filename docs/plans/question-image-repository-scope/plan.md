---
created: 2026-11-16
status: done
requirements:
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-004
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
legacy_specs: []
---

# Fix plan: Question image repository scope

## Overview

Correct [issue #5329](https://github.com/kdlbs/kandev/issues/5329) with a small
conditional in the existing image reader and clearer tool guidance. One work
order owns the fix, focused regressions, and a public guide correction.
Implementation is complete.

Tasks owns the saved image-reference contract. The existing
[requirements](../../specs/tasks/requirements/question-image-previews.md) and
[design](../../specs/tasks/system-design/question-image-previews.md) remain authoritative.

## Evidence and root cause

Baseline: `b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`, matching remote main
on 2026-11-16. The issue had no comments or image attachments and is assigned
to `carlosflorencio`.

The image reader constructs `workspace / repo / path`. In a single-repository
session, the workspace already is the repository directory. Supplying its name
as `repo` repeats that directory and produces `not_found`.

A temporary `TestRepro5329QuestionImageSelfRepository` drove the real reader.
The omitted-`repo` control passed. The exact repository-name case failed with
`not_found`. This command reproduced the report and exited 1 as expected:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^TestRepro5329QuestionImageSelfRepository$' -v -count=1 -timeout=3m)
```

The temporary test was removed after diagnosis.

## Scope

### In scope

- One scope-selection conditional in `openQuestionImageRoot`.
- Shared question/choice schema text and the existing public image-question guide.
- Focused reader, HTTP endpoint, and schema regressions.

### Out of scope

New resolver abstractions, caches, runtime fields, routes, creation-time image
preflight, provider changes, and frontend changes.

## Technical approach

Keep literal child selection first. Treat `repo` as the workspace root only
when all three conditions hold:

1. The workspace root has its own `.git` entry, using `workDirHasOwnGitEntry`.
2. `repo` exactly matches the configured workspace folder name.
3. The same-name child entry is absent, using `os.Lstat` and `os.ErrNotExist`.

The existing reader handles the selected root afterward. A real same-name child
keeps precedence. Its missing image or failed link never triggers another lookup.
Existing root handles continue to enforce containment.

Keep this conditional in `question_images.go`. Do not change tracker discovery
or saved references. The own-`.git` helper already handles repository directories
and worktree files without a Git subprocess.

Clarify `path` and `repo` in the shared MCP image schema. Tell agents to omit
`repo` when the workspace itself is the repository. Add the same explanation to
`docs/public/agent-communication.md`.

The issue accepts this alias and guidance correction. Creation-time preflight
changes the established read-time error contract and is unnecessary for this fix.

### Compatibility

| Caller/runtime | Expected behavior | Evidence or fallback |
| --- | --- | --- |
| Operator questions and choices, current local or remote agentctl | Same saved-reference reader and authenticated endpoint | Reader and HTTP regressions. No provider-specific changes |
| Real child or attached source | Existing selection and containment | Precedence cases and existing source/symlink tests |
| Older retained agentctl | Correct omitted-`repo` calls work | Follow the clarified guidance until runtime update |

## Tests

Use one new focused process test file and existing fixtures.

| Acceptance | Evidence |
| --- | --- |
| AC-TASKS-QUESTION-IMAGES-002.6 | `TestQuestionImageSelfRepositoryScope`: omitted and exact own-name scopes return identical bytes. Unknown scopes and a bare workspace do not gain an alias |
| AC-TASKS-QUESTION-IMAGES-002.1, 002.3, 002.6 | `TestQuestionImageRepositoryScopePrecedence`: distinct child bytes win. A missing child image or broken/escaping child symlink never falls back |
| AC-TASKS-QUESTION-IMAGES-002.6 | Extend `TestQuestionImageEndpoint` with an own-name request against a root Git repository |
| AC-TASKS-QUESTION-IMAGES-004.5 | `TestQuestionImagesSchemaExplainsRepositoryScopes`: question and choice schema guidance explains both layouts |

Reuse existing containment, workspace rebind, source allowlist, and missing-file
tests. Do not add new lifecycle test matrices for this conditional.

## E2E tests

Run the existing desktop and phone image-question specs as compatibility checks.
The focused HTTP regression proves the changed path through the real endpoint.
Existing browser flows cover loading, inspection, reload, and answers. There are
no frontend edits or new browser fixtures.

The existing desktop clarification panel and full-height phone inspector remain
unchanged. No new mobile composition or UI preview is necessary.

## Work orders

- [x] [Task 01: Repair question image repository scope](task-01-repair-repository-scope.md) (done)

Execute sequentially. Task 01 contains exact commands. A later explicit
implementation request starts TDD.

## Verification results

The permanent regression test reproduced the issue before the fix: omitted scope
passed, while the exact workspace-folder alias returned `not_found`. The endpoint
and MCP schema regressions also failed before their changes.

Implementation and verification passed:

- `go test -trimpath -race ./internal/agentctl/server/process -run '^TestQuestionImage' -count=1 -timeout=3m`
- `go test -trimpath ./internal/agentctl/server/api -run '^TestQuestionImage' -count=1 -timeout=3m`
- `go test -trimpath ./internal/mcp/server -run '^Test(QuestionImages|ChoiceImages)' -count=1 -timeout=3m`
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/question_images.go apps/backend/internal/agentctl/server/process/question_images_repo_scope_test.go apps/backend/internal/agentctl/server/api/question_images_test.go apps/backend/internal/mcp/server/question_images.go apps/backend/internal/mcp/server/ask_user_question_images_test.go`: 0 issues.
- `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/question-image-previews.spec.ts --retries=0`: backend and Vite assets built; 4 passed.
- `pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts --retries=0`: 3 passed.
- Public docs tests: 63 passed; public docs validation: 55 pages.
- Specification catalog: 452 decisions and 2014 specifications validated.
- Specification linter tests: 36 passed; all specification files passed.
- PR documentation coverage preflight: covered, no errors.
- `git diff --check`: passed.

Design-package validation passed: catalog validation, all 36 specification-linter
tests, specification lint, PR-documentation traceability preflight, and whitespace
checks. The package and implementation remain unstaged and uncommitted.

The public image-question guide and shared MCP schema now describe workspace and
child repository scope. Earlier image packages remain completed historical records.

## Risks

A broad basename rewrite can hide a real child. The absent-child condition avoids
that ambiguity. An older retained runtime needs corrected calls or an update.
