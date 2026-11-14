---
id: "01-return-selected-labels"
title: "Return readable labels with selected option IDs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-ANSWER-LABELS-001
acceptance_criteria:
  - AC-TASKS-QUESTION-ANSWER-LABELS-001.1
  - AC-TASKS-QUESTION-ANSWER-LABELS-001.2
  - AC-TASKS-QUESTION-ANSWER-LABELS-001.3
  - AC-TASKS-QUESTION-ANSWER-LABELS-001.4
system_design:
  - ../../specs/tasks/system-design/user-question-answer-labels.md
---

# Task 01: return readable labels with selected option IDs

## Summary

Implement the answer projection and discovered output schema together.
Use TDD to reproduce missing labels before changing production behavior.

## In scope

- Enrich answers using the matching normalized question and exact option ID.
- Add the optional schema field and concise tool description.
- Add the test cases and protocol journey listed in the plan.
- Update the public operator-question reference and record actual check results.

## Out of scope

UI, input schema, native providers, parent questions, external answer tools,
persistence, response validation, delivery lifecycle, and unrelated refactoring.

## Acceptance

1. All four linked acceptance criteria pass through focused regression tests.
2. Streamable HTTP discovery and results expose matching IDs and exact labels.
3. Existing question tests pass, and public documentation describes the result.

## Verification

Run from the repository root. First add the failing regression test and record
its failure. Then implement and run the complete block.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/server -run 'Test(AskUserQuestionAnswerLabels|ExtractQuestionAnswerLabels)' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/mcp/server -count=1)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/handlers.go apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/ask_user_question_answer_labels_test.go apps/backend/internal/mcp/server/tool_metadata_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the normal Go cache when allowed. If sandbox cache writes fail, request the
necessary command permission or use a task-owned writable cache. A cache error
does not establish a regression result.

## Files likely touched

- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/tool_metadata_test.go` (existing exact-result expectation)
- `apps/backend/internal/mcp/server/ask_user_question_answer_labels_test.go` (new)
- `docs/public/agent-communication.md`
- This work order, its plan, and the paired requirement and design status fields.

## Dependencies

None. The user authorized implementation on 2026-10-24.
Check current main for a competing fix before starting.

## Risks

Do not infer labels from generated-ID syntax. Do not copy a label from another
question. Keep rejection and custom-text behavior intact. Avoid adding a new
validation rule for malformed or duplicate input IDs in this repair.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/user-question-answer-labels.md).
- [System design](../../specs/tasks/system-design/user-question-answer-labels.md).
- [Plan and reproduction evidence](plan.md).
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- Existing registered-tool and Streamable HTTP tests in
  `apps/backend/internal/mcp/server/ask_user_question_test.go`.

## Results

Completed on 2026-10-24 after the user's explicit implementation request.

The initial focused run failed on absent labels and schema metadata (0.106s).
After the implementation, all four focused test functions and ten projection
subcases passed (0.077s). The HTTP test exercised initialize, discovery, and
an A/B/C tools/call result through the actual Streamable HTTP server.

Validation commands, run from the directories specified above:

```text
go test -trimpath -tags fts5 ./internal/mcp/server -run 'Test(AskUserQuestionAnswerLabels|ExtractQuestionAnswerLabels)' -count=1 -v
PASS (0.077s)
go test -trimpath -race -tags fts5 ./internal/mcp/server -count=1
PASS (52.392s)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/handlers.go apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/ask_user_question_answer_labels_test.go apps/backend/internal/mcp/server/tool_metadata_test.go
PASS (0 issues)
node --test scripts/validate-public-docs.test.mjs
PASS (1 test)
node scripts/validate-public-docs.mjs
PASS (48 published pages)
python3 scripts/list-docs.py validate
PASS (384 decisions, 1618 specifications)
python3 scripts/lint-spec-files.test.py
PASS (36 tests)
python3 scripts/lint-spec-files.py --all
PASS
git diff --check
PASS
```

The shell lacked `golangci-lint`. The lint rerun prefixed PATH with the existing
`/tmp/kandev-builtin-lint-qt_g9x0v/golangci-lint-2.9.0-linux-amd64` directory.
The binary reported version 2.9.0, matching the repository's CI pin.
Go and lint used authorized access to the shared caches.

The repository documentation-coverage validator accepted the actual production
paths and this package with status `covered` and no errors.
Main remained `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab` at implementation entry.
Open PR #4727 adds answer provenance in separate files and does not duplicate
this response-label change.

Public reference updated: `docs/public/agent-communication.md`.
Requirements and design match the implementation and are now active/current.
The initial backend tests simulated the answer source. The implementation was
later committed and published as PR #4752 at the user's request. Live provider
interpretation remains unverified.


## PR fixup validation

CI exposed an order-sensitive JSON assertion in the shared Threads question
helper. All six desktop and two phone failures received the expected IDs and
new labels. A focused local keyboard run reproduced the same failure. The
helper now parses one answer receipt and checks each question's ID and label.

CI also exposed two test synchronization and selector issues. The branch-switch
test now waits for picker focus restoration before opening the next picker.
The mobile live-scroll test scopes its markers to agent messages, so the user
prompt cannot satisfy or duplicate the reply locator. No UI behavior changed.

Validation uses an isolated merge of the PR with current main. The MCP package
passed with the race detector. All 13 desktop composer cases passed across the
main run and a four-case native-zoom rerun. The first zoom run could not launch
Chromium because host libraries were absent; the rerun supplied existing Nix
libraries. The phone answer and live-scroll cases passed three times each
(9 tests, retries disabled). The branch-switch case passed ten repetitions
with retries disabled. Focused ESLint and formatting checks passed.

The managed E2E build used `MAKEFLAGS=GOFLAGS=-buildvcs=false` for its temporary
worktree. Runs used loopback fixture settings and the host's browser libraries.
Subsequent `--no-build` runs reused those exact production artifacts.
Browser commands (from `apps/web` in the isolated merge checkout):

```text
pnpm e2e:run --host --no-build --project chromium e2e/tests/task/threads-composer-disclosure.spec.ts --retries=0
pnpm e2e:run --host --no-build --project chromium e2e/tests/task/threads-composer-disclosure.spec.ts --grep 'scrolls long required questions' --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-threads-composer-disclosure.spec.ts e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts --grep 'scrolls and submits a long required question by touch|follows a live turn from the bottom' --retries=0 --repeat-each=3
pnpm e2e:run --host --no-build --project chromium e2e/tests/task/create-task-branch-selector.spec.ts --grep 'switching worktree.*local resets the chip' --retries=0 --repeat-each=10
```

The next CI run exposed the same focus-restoration race in the Jira project
filter and the remote repository picker. Both tests now wait for the closing
popover to restore focus before opening another popover. The two repaired
flows passed five repetitions each with retries disabled and CPU affinity
limited to two cores. Focused ESLint and formatting checks passed.

An unchanged fork-PR comparison recovery test also failed once in CI and passed
on retry. The full two-case spec passed five local repetitions without retries.
Its CI process group then passed with two-core affinity: 22 desktop and 9 phone
tests, run separately because the managed runner accepts one project per run.
The local runs did not reproduce the failure. No speculative source change was
made for that result. A fresh remote run must establish its CI outcome.

Final remote CI and review status remain external to this implementation record.
