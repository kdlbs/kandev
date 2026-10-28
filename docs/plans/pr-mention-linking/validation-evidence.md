# Design-package validation evidence

Date: 2026-10-09. Scope: documentation and source investigation only.
This record covers the improved package in the destination task workspace at HEAD `3fed5570cec533f468c25ed03c84967bdc972588`.
The package contains eleven documents, four work orders, seven requirements, and 33 acceptance criteria.
The revision defines immutable identity, selection IDs, nullable plugin fences, and uncertain result state across requirements, designs, work orders, and regression scenarios.
Catalog counts cover the repository. They do not represent package files or product test results.

## Results at design-package validation

| Check | Result |
| --- | --- |
| `python3 scripts/list-docs.py validate` | Passed: 368 decisions and 1499 specifications. |
| `python3 scripts/lint-spec-files.test.py` | Passed: 36 tests. |
| `python3 scripts/lint-spec-files.py --all` | Passed: all specification files. |
| Integration catalog discovery | The requirements and both designs appear under Integrations. |
| Repository PR-documentation preflight | Actual documentation-only file set: exempt and valid. |
| Contract-graph preflight | Covered: all four work orders validate against the plan, requirements, ACs, and design. Zero errors. |
| Local Markdown target check | Passed for all eleven documents. Removed the dependency on source-workspace-only t3code analysis. |
| Requirement/AC coverage inventory | Seven requirements and 33 acceptance criteria; every AC appears in at least one work order. |
| Whitespace and status checks | Passed; eleven package files are untracked/unstaged, with no tracked source changes. |

The contract-graph preflight used the repository's `validateCoverage` export with an in-memory synthetic source-change entry.
That entry forces validation instead of the documentation-only exemption. It did not modify source, stage a file, or claim implementation coverage.

## Reproduce the contract and link checks

Run from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node <<'JS'
const fs = require('fs');
const path = require('path');
const validator = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/pr-mention-linking';
const paths = fs.readdirSync(root).filter(p => p.endsWith('.md')).map(p => `${root}/${p}`);
paths.push('docs/specs/integrations/requirements/task-change-registration.md');
paths.push('docs/specs/integrations/system-design/task-change-registration.md');
paths.push('docs/specs/integrations/system-design/task-change-registration-plugins.md');
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: 'added', additions: 1, changes: 1}));
const docs = validator.validateCoverage({changedFiles, fileContents});
const graph = validator.validateCoverage({
  changedFiles: [...changedFiles, {
    filename: 'apps/backend/internal/backendapp/task_change_link_coordinator.go', status: 'modified'
  }],
  fileContents
});
console.log({docsOnly: docs.status, graph: graph.status, workOrders: graph.workOrders, errors: graph.errors});
if (!docs.ok || !graph.ok) process.exitCode = 1;
const broken = [];
for (const [file, text] of Object.entries(fileContents)) {
  for (const match of text.matchAll(/\]\(([^)]+)\)/g)) {
    const target = match[1].split('#')[0];
    if (!target || /^(https?:|app:)/.test(target)) continue;
    if (!fs.existsSync(path.resolve(path.dirname(file), target))) broken.push({file, target});
  }
}
console.log({brokenLocalLinks: broken});
if (broken.length) process.exitCode = 1;
JS
git diff --check -- docs/plans/pr-mention-linking docs/specs/integrations/requirements/task-change-registration.md docs/specs/integrations/system-design/task-change-registration.md docs/specs/integrations/system-design/task-change-registration-plugins.md
git status --short --untracked-files=all -- docs/plans/pr-mention-linking docs/specs/integrations/requirements/task-change-registration.md docs/specs/integrations/system-design/task-change-registration.md docs/specs/integrations/system-design/task-change-registration-plugins.md
```

`git diff --check` does not inspect untracked file contents. A separate in-memory scan checked trailing whitespace and final newlines for these eleven files.
Local link checks cover target existence, not external URL availability. Pinned t3code source was available locally at the exact requested revision.

## Scope and lifecycle at design-package validation

The source workspace contained unrelated t3code analysis and a delegated-work package. The copy included only these eleven PR-linking documents.
The improvement changes only those documents. It preserves their draft/pending status and does not alter production files or permanent tests.
No product test suite, database migration, browser run, live PR mutation, commit, push, merge, or deployment occurred.
Planned test filenames and commands in the work orders describe later implementation evidence.

At design-package validation time, requirements, both designs, and the plan were `draft`, and all work orders were `pending`. There were no material unanswered product questions.

## Implementation evidence through Task 02 (2026-10-10)

Tasks 01 and 02 are complete. Tasks 03 and 04 remain pending, so the shared requirement and system-design documents remain `draft` as required by the work orders.

| Check | Result |
| --- | --- |
| Focused backend reference/admission tests | Passed for `entityrefs`, `mentions`, task handlers, task service, and SQLite persistence. |
| PostgreSQL registration/admission tests | Passed with `-race` against disposable PostgreSQL 16; the database container was removed after the run. |
| Focused frontend tests | Passed: 209 tests across eleven files covering references, composer registration, admission routing, provider events, revision fencing, and state. |
| TypeScript | `pnpm run typecheck` passed. |
| Localization | `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. |
| Desktop E2E | Two cases passed in Chromium for direct-send and initial-brief admission; the managed run rebuilt the backend and Vite test assets. |
| Mobile E2E | One case passed in mobile Chrome for the registration drawer and accepted message. |
| Documentation and repository checks | Specification catalog/lint, public-doc tests and validation, and `git diff --check` passed. |

These results cover the completed direct and initial-message slice. Queue edit admission, plugin conformance, and the final package-wide checks remain assigned to Tasks 03 and 04.

### Static review remediation evidence (2026-10-10)

The focused review identified five boundary/race regressions; each now has a matching guard and regression coverage:

| Finding | Remediation and regression evidence |
| --- | --- |
| Background processing could act after the admitting actor lost access. | The processor resolves the persisted actor again, establishes an actor-scoped context, and checks current task/workspace/repository access before provider dispatch and mutation. GitHub and GitLab tests revoke access before processing and at the mutation boundary and verify no association is written. |
| A lost unlink event left a completed result appearing linked. | Startup and periodic reconciliation compare completed registration results with canonical provider state. GitHub and GitLab restart tests drop detach delivery and verify the message is unlinked with a fresh relink action. |
| Delayed association events could resurrect or remove a newer browser row. | Committed revisions flow through provider DTOs, WebSocket handlers, stores, and hydration. GitHub and GitLab tests cover delete N+1 followed by update N and relink N+2 followed by delete N+1, including reconnect hydration. |
| PostgreSQL JSON metadata writers could overwrite one another. | All message registration metadata read/modify/write paths acquire the same row lock before reading. A barrier-controlled, multi-connection PostgreSQL test updates separate selections concurrently and verifies both states/IDs and unrelated metadata. |
| Queue admission displayed an unsupported link-on-send promise. | The composer exposes linking only for supported direct acceptance; queue/clarification admission is context-only. Admission sanitizes a direct draft that races into a queue. Tests cover busy-session and clarification paths plus the direct-to-queue race. |

Validation for these remediations: focused Go registration/reference tests across `entityrefs`, `mentions`, task handlers/service, SQLite, backend app, GitHub, and GitLab; the PostgreSQL concurrency/admission tests with `-race`; 209 focused frontend tests and TypeScript typecheck; desktop Chromium (2/2) and mobile Chrome (1/1) managed browser cases; localization, public-doc, specification-catalog, and whitespace checks. The E2E scenarios rebuild the backend and production Vite assets. These are focused package and scenario checks, not a claim that the full backend or web test suites ran.

## Implementation evidence through Task 03 (2026-10-26)

Tasks 01 through 03 are complete. Task 04 and promotion of the shared requirements and system designs remain pending; those documents stay `draft`.

| Check | Result |
| --- | --- |
| Backend queue packages | Passed `go test -trimpath -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers -count=1`. |
| Backend registration packages | Passed focused orchestrator, task-handler, and SQLite repository tests with `-run 'Registration|Queue|InitialTaskBrief|EntityReference'`. |
| PostgreSQL queue and registration | Passed race-enabled queue, task repository, and backend application tests against disposable PostgreSQL 16, including queue admission/edit intent transactions and concurrent message metadata writers. |
| Backend build and lint | Passed `make build` and `make lint` in `apps/backend`. |
| Focused frontend tests | Passed: 175 tests across ten files for queue admission/edit selection, provider state, message references, and registration routing. |
| Web checks | Passed `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`. |
| Production web build | Passed `pnpm --filter @kandev/web build:vite`. |
| Desktop E2E | Passed: 9 Chromium cases for entity reference composer, direct and initial-message admission, and queued reselection/save. |
| Mobile E2E | Passed: 4 mobile Chrome cases for entity-reference selection, accepted-message registration, and queued edit/reselection. |
| PR screenshots | Captured current desktop and mobile queue-edit states from the rebuilt bundle; both files pass PNG metadata checks and remain ignored under `apps/web/.pr-assets/`. |
| Documentation and whitespace | Specification catalog/lint, public-doc tests/validation, and `git diff --check` passed. |

The queue integration tests run the same admission and edit transaction assertions in SQLite and PostgreSQL. They cover replay without duplicate intents, rollback after an injected producer failure, unchanged Save, fresh reselection, accepted removal, fold and dispatch propagation, clear, Send now, and registration status projection. Browser tests confirm that a reselection's identity and prompt survive reload and desktop/phone breakpoint changes, and only Save creates the association. These are targeted work-order checks; Task 04 conformance and final package-wide checks remain open.

### PR review fixup evidence (2026-10-26)

The PR branch was rebased onto `main` at `5a0e976d4842a5344cd5d099618898c7769e2634`. Resolving the TipTap editor conflict retained upstream's structured `textToEditorContent` parser and the registration-capability synchronization. The review fixes preserve Task 03 scope. Task 04 and shared requirement/design promotion remain pending.

| Finding | Fix and evidence |
| --- | --- |
| Processor restart could reuse a closed worker signal. | Each worker generation owns its completion channel. Stop waits for that worker before another start. Backend processor restart tests pass. |
| URL-only unlink and replacement could ignore or contradict asserted identity. | Both operations resolve the URL and compare every supplied provider, repository, and number field before mutation. Backend coordinator tests pass. |
| Successful intents could miss the association revision or reuse the first-origin ID as an operation receipt. | Already-active links persist their current revision. The original operation ID remains separate from the association's first-origin metadata. Backend registration tests pass. |
| GitLab manual relink could fail on a detached row, report revision zero, or race with detach during sync. | Manual relink restores the row with an expected revision and keeps its metadata. Upserts return the committed revision, and sync handles a missing row. GitLab service and store tests pass. |
| Draft restoration and reference parsing could retain stale or invalid state. | The editor consumes saved drafts once, clears them on Cancel, omits nullable TipTap attributes, and uses the shared UUID helper. Focused frontend tests pass. |
| Reference controls could have weak pointer or touch affordances. | Queue and reference actions use pointer cursors and coarse-pointer targets of at least 44 px. Focused component tests and frontend lint pass. |
| A queue session-switch E2E counted an unsaved edit as a normal row. | The editor resumes the draft after task navigation. The desktop and mobile tests now assert the restored value and save it. Both managed E2Es pass against the fresh production build. |
| Provider refresh events could carry revision zero while the database stored revision one. | `ReplaceTaskPR` now returns the persisted default revision and preserves it during provider refresh. A store regression test and the affected desktop/mobile PR status E2Es pass. |
| The plan-mode queue E2E could lose its busy-session precondition before submit. | The mock turn now stays active for 30 seconds. The exact test passed twice with retries disabled. |

Post-rebase validation passed: `make build` and `make lint` in `apps/backend`; focused Go registration tests across backendapp, entityrefs, GitHub, GitLab, MCP, messagequeue, task handlers, and SQLite; 310 frontend tests across 16 files; web typecheck, lint, localization checks, and new-code ratchet; specification and public-doc validators; and desktop Chromium and mobile Chrome queue-switch E2Es. The GitLab package also passed its full unit suite. The PostgreSQL test DSN was not available in this pass. Earlier Task 03 evidence records successful race-enabled tests against disposable PostgreSQL 16. The pushed PR's PostgreSQL matrix remains the current post-rebase check.

The PR run on pre-fix head `b9ca1219b22` reported eight failed E2E shards before the revision-return and queue-fixture fixes. The affected PR status and queue cases were reproduced and passed in focused managed runs against the patched tree with retries disabled; the queue case passed twice. The pre-fix run's rollup jobs were still queued during this follow-up, so they do not validate the patched head.

The PR run on `9c1b8f0a1f7` exposed one flaky result in the existing workflow agent-switch scenario. The test waited for the reused profile-A session and profile-B session separately, then read their final states from a later snapshot; CI captured profile A as `RUNNING` at that point. The test now polls both expected lifecycle states together before asserting the final snapshot. The exact scenario passed five consecutive managed Chromium runs with retries disabled after this change. The old-head blob audit found 118 reports, no parse errors, and one failed result with one retry attempt; it does not validate the rebased head.

After GitHub reported a merge conflict from later `main` advancement, the branch was rebased onto `5a0e976d4842a5344cd5d099618898c7769e2634`. Post-rebase validation passed 65 focused TipTap tests, TypeScript typecheck, and the desktop and mobile PR mention-linking E2E cases (2/2) against a fresh production build. Hosted checks for the rebased head remain the final PR validation.

On 2026-10-28, the branch was rebased again onto `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`. The workflow-agent-switch conflict was resolved by retaining main's lifecycle-pending guard and polling all final session states together. The four affected workflow lifecycle scenarios and the web typecheck pass locally on the rebased tree. The hosted run for the prior head completed with 118 reports and no parse errors. Three existing E2E tests failed on their first attempts and passed on retry: sidebar touch-target geometry, mobile navigation search behavior, and mobile notification-event persistence. The retry summary recorded zero ultimately failed tests, but the E2E gate rejects any retry. The PR walkthrough generator also timed out at its deadline. These prior-head results do not validate the newly rebased head. Task 04 and shared requirement/design promotion remain pending.
