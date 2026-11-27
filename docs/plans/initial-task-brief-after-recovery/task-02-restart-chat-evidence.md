---
id: "02-restart-chat-evidence"
title: "Prove restart behavior in Chat"
status: done
wave: 2
depends_on: ["01-ready-session-admission"]
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.1
  - AC-TASKS-INITIAL-TASK-BRIEF-001.2
  - AC-TASKS-INITIAL-TASK-BRIEF-001.3
  - AC-TASKS-INITIAL-TASK-BRIEF-001.7
  - AC-TASKS-INITIAL-TASK-BRIEF-001.11
  - AC-TASKS-INITIAL-TASK-BRIEF-001.12
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
---

# Task 02: Prove restart behavior in Chat

## Summary

Extend the existing desktop and phone initial-brief tests with a prompt-free
backend restart before first submission. Prove the same recovered session stores
one combined first prompt, retains it after reload, and dispatches an ordinary follow-up.

## In scope

- Retain both existing prepared-session E2E scenarios; add
  `keeps the first brief after prompt-free recovery` in both spec files.
- Extend the shared helper with the worker `backend` restart dependency and
  deterministic session/environment readiness checks. Use only the isolated
  worker fixture and production recovery path.
- Preserve and restore the isolated user's existing
  `prevent_auto_start_agent_on_open` preference around page recovery, so an
  automatic browser resume does not race the first direct message.
- Before restart, wait for prepared workspace completion. After opening the
  task, assert the same session is WAITING_FOR_INPUT with an agent execution
  and zero stored user prompts before interacting with the composer.
- Submit the instruction, compare the combined stored prompt and rendered
  bubble, reload, then send a second instruction. Assert no repeated brief,
  no extra user row, unchanged task description, and no document overflow.
- Retain the existing processed-message regression; change its test only if
  recovered-session data requires a distinct fixture. Do not change fallback logic.
- Apply `/docs-maintainer` after successful product checks to clarify the
  first-message explanation in `docs/public/tasks-and-workflows.md` (an explanation
  section in the existing task guide). Search README/screenshots for conflicts.
- Record exact test discovery, results, skips/blockers, and documentation
  validation in both work orders and this package's manifest.

## Out of scope

No new controls, layout, synthetic transcript policy, provider probes, runtime
API, live-user database manipulation, backfill, or website publication.

## Acceptance

- Desktop `chromium` and phone `mobile-chrome` each prove the restart case and
  the retained CREATED case; restart coverage must establish WAITING_FOR_INPUT
  before submission instead of accidentally exercising the old path.
- Prompt #1 contains brief then instruction once in storage and Chat, survives
  reload, and the second prompt contains only its own instruction.
- Public wording describes the implemented recovery boundary; all package
  validation has actual results, and skipped PostgreSQL coverage remains visible.

## ASCII UI preview

UI-01 from [the package preview](plan.md#ascii-ui-preview), covering
`AC-TASKS-INITIAL-TASK-BRIEF-001.2`, `.7`, `.11`, and `.12`:

```text
UI-01: Chat after first message on a recovered session

+---------------------------+
| User #1                   |
| Original task brief       |
|                           |
| Rebase on the target PR.   |
| Agent receives both       |
+---------------------------+
| Composer           [Send] |
+---------------------------+
```

Desktop retains its Chat pane; phone uses the existing full-height Chat
destination and vertical transcript scroll owner. Fixed composer and safe-area
behavior stay with the current mobile exemplar. The structural check is one
combined first prompt, followed by ordinary messages, with content reachable
through existing controls. ASCII borders and spacing are illustrative.

## Verification

Run from repository root after Task 01. In a fresh worktree, install dependencies
once before any pnpm command. Do not overlap the two managed E2E commands.
Use `/e2e` for red evidence against the pre-fix revision when feasible; Task 01's
required red regression remains the primary defect proof.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-processed-messages-fallback.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/initial-task-brief.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-initial-task-brief.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/tests/chat/initial-task-brief.spec.ts e2e/tests/chat/mobile-initial-task-brief.spec.ts e2e/tests/chat/initial-task-brief-helpers.ts)
(cd apps/web && pnpm run e2e:sleep-ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node -e 'const fs = require("node:fs"); const path = require("node:path"); const cp = require("node:child_process"); const v = require("./.github/scripts/pr-docs.cjs"); const dir = "docs/plans/initial-task-brief-after-recovery"; const docs = [...fs.readdirSync(dir).map(n => path.join(dir, n)), "docs/specs/tasks/requirements/initial-task-brief.md", "docs/specs/tasks/system-design/initial-task-brief.md"]; const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, "utf8")])); const changedFiles = [...new Set([...cp.execFileSync("git", ["diff", "--name-only", "-z", "HEAD"], {encoding: "utf8"}).split("\0"), ...cp.execFileSync("git", ["ls-files", "--others", "--exclude-standard", "-z"], {encoding: "utf8"}).split("\0")].filter(Boolean))]; const result = v.validateCoverage({changedFiles, fileContents}); console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}, null, 2)); process.exitCode = result.ok ? 0 : 1;'
git diff --check
git status --short -- docs/plans/initial-task-brief-after-recovery
```

The Node command runs the repository documentation coverage preflight through
the exported `validateCoverage` function with actual changed and untracked paths.
Run it before committing. Validate the implementation's actual changed paths
at completion; the design
handoff's prospective-path result does not prove final PR coverage.

## Files likely touched

- `apps/web/e2e/tests/chat/initial-task-brief.spec.ts`
- `apps/web/e2e/tests/chat/mobile-initial-task-brief.spec.ts`
- `apps/web/e2e/tests/chat/initial-task-brief-helpers.ts`
- `apps/web/hooks/use-processed-messages-fallback.test.ts` (only if a distinct fixture is needed)
- `docs/public/tasks-and-workflows.md`
- `docs/plans/initial-task-brief-after-recovery/plan.md`
- `docs/plans/initial-task-brief-after-recovery/task-01-ready-session-admission.md`
- `docs/plans/initial-task-brief-after-recovery/task-02-restart-chat-evidence.md`

## Dependencies

Task 01 must pass its targeted backend checks. Do not restate prior package's
completed checks as results for these new restart cases.

## Risks

- A browser-idle wait alone does not prove recovered WAITING_FOR_INPUT state.
- Restart must preserve the fixture database and correlate the original session.
- Spec files must be discovered in the correct project; report actual counts.
- The mobile case proves content and reachability, without changing the mobile composition.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/initial-task-brief.md).
- [Design](../../specs/tasks/system-design/initial-task-brief.md).
- [Evidence, preview, and scenario matrix](plan.md).
- `apps/web/e2e/fixtures/backend.ts`, `ApiClient.listTaskSessions`,
  `ApiClient.listSessionMessages`, and `SessionPage`.
- [Public documentation guide](../../public/README.md).

## Results

The desktop and mobile project commands each passed both scenarios (2 tests
each), including the restart case. The restart helper uses the production
prompt-free resume path, proves the same session is ready with no accepted user
prompt before and after backend restart, and restores the previous auto-start
preference.

`pnpm run typecheck`, changed-file ESLint, the processed-message fallback Vitest
file (8 tests), and `pnpm run e2e:sleep-ratchet` passed. The E2E runner built the
web assets for both projects. Public-doc checks passed (62 validator tests and
47 published pages); specification validation covered 360 decisions and 1427
specifications, the spec linter's 36 tests passed, and all specification files
passed. Changed-path PR-doc coverage returned `covered` with no errors, and
`git diff --check` passed.
