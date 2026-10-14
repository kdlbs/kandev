---
id: "01-preserve-nested-adf-paragraphs"
title: "Preserve nested ADF paragraph boundaries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.2
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.4
system_design:
  - ../../specs/integrations/system-design/jira-watcher-task-prompts.md
---

# Task 01: Preserve Nested ADF Paragraph Boundaries

## Summary

Repair the shared Jira converter so paragraphs nested under lists or other
containers remain distinct in fetched tickets and watcher search results.
Independently reproduce both HTTP client failures before the production change,
then prove exact outputs and compatibility with the complete affected package.

## In scope

- Add two causal real-HTTP regressions and table-driven converter controls.
- Repair `extractDescription`/`walkADF` locally in `cloud_client.go`.
- Preserve top-level spacing, inline adjacency, explicit breaks, plain/null
  values, and the converter's existing tolerant behavior.
- Record completed task-defined validation in this work order and its plan.

## Out of scope

The plan's renderer, provider API, authentication, watcher/task schema, UI,
dependency, feature-toggle, and broad-refactor exclusions apply. ROOT's private
proof source and all foreign resources remain outside file ownership.

## Acceptance

1. Both named HTTP regressions independently fail on unchanged production
   because instruction paragraphs fuse, then pass with the exact expected
   `Run migration.\n\nVerify the schema.\n\nRestart worker.` string.
2. Paragraph boundaries survive sibling items, multiple paragraphs per item,
   nested ordered/bullet lists, blockquote, and unknown enclosing containers.
   Inline fragments remain adjacent and every compatibility control passes.
3. The complete Jira race package, staged affected-package lint, catalog/spec
   validation, documentation-coverage preflight, and diff gates pass with actual
   joined exits recorded. Mark the plan and work order done before publication.

## Regression matrix

Use separately authored JSON HTTP fixtures through each production client,
without replacing or mocking `extractDescription`. Serve ticket transitions
explicitly and confirm the watcher fixture is returned by its search endpoint.
Reuse `newMockServer` for cleanup and use literal expected strings.

`TestExtractDescription_NestedParagraphs` covers:

- Three sibling list items with one paragraph each.
- Two paragraphs within one list item and a following sibling item.
- A paragraph, a nested list's paragraphs, and a following outer paragraph.
- Multiple blockquote paragraphs and paragraphs inside an unknown wrapper.

`TestExtractDescription_Compatibility` covers:

- Plain string including surrounding whitespace: byte-for-byte unchanged.
- Null/absent description and a scalar unsupported shape: empty string.
- Adjacent text fragments with marks: `Run ` + `migration.` becomes
  `Run migration.` with no inserted newline or stripped space.
- Hard and soft breaks inside a paragraph: one newline per break.
- Top-level paragraphs: exactly two intervening newlines; a trailing explicit
  break in the first paragraph retains its additional newline.
- Literal interior spaces and consecutive newlines: retained verbatim.
- Top-level empty paragraph between readable paragraphs: existing four-newline
  spacing retained. Empty document, empty list, and lone empty paragraph: empty.
- Empty nested paragraphs: no extra blank rows, while readable paragraphs on
  either side remain separated.
- Non-object children, wrong-type `content`/`text`, and missing fields: no
  panic; readable siblings retain source order and separation. Preserve the
  existing top-level separator contribution from ignored document children.
- Unknown containers with adjacent raw inline text and no paragraphs: existing
  concatenation retained. Unsupported mention/media leaves remain empty.

## Execution sequence

1. Wait for the explicit implementation release and ROOT's global heavy lease.
   Mark this work order and plan in progress. Use the selected primary session;
   no delegates or new sessions.
2. Prepare the requested Go 1.26 and Node 24.18/pnpm 9.15.9 PATH without changing
   shared caches or toolchains. In a fresh worktree, install workspace
   dependencies once with `(cd apps && pnpm install --frozen-lockfile)` under
   the lease before tests, lint, or hooks. No install belongs to design.
3. Add only the owned permanent tests. Run each HTTP regression independently
   against unchanged production and retain the actual causal RED results.
   Run compatibility controls separately and expect success.
4. Implement the minimum recursive paragraph-boundary correction. Run the
   focused GREEN matrix and complete affected package below.
5. Stage owned Go files before changed-package lint so its baseline includes
   the new test. Complete document gates, synchronize statuses/results before
   first publication, and follow the authorized delivery workflow with normal
   active hooks. Preserve unrelated edits and resources throughout.

## Verification

Run from repository root with the prepared toolchain PATH. Every heavy command
needs the granted global lease. The two RED commands intentionally return 1
before the production repair; run and join them separately rather than hiding
their failures in a successful shell wrapper. Run the control command on RED
and again within GREEN. Do not rerun a completed pass without a relevant change.

```bash
# RED: unchanged production, each client path independently.
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=240s ./internal/jira -run '^TestCloudClient_GetTicket_NestedDescriptionParagraphs$')
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=240s ./internal/jira -run '^TestCloudClient_SearchTicketsForWatch_NestedDescriptionParagraphs$')
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=240s ./internal/jira -run '^TestExtractDescription_Compatibility$')

# GREEN: after repair, focused matrix followed by complete affected package.
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=240s ./internal/jira -run '^Test(CloudClient_(GetTicket|SearchTicketsForWatch)_NestedDescriptionParagraphs|ExtractDescription_(NestedParagraphs|Compatibility))$')
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=240s ./internal/jira)

gofmt -w apps/backend/internal/jira/cloud_client.go apps/backend/internal/jira/adf_description_test.go
git add apps/backend/internal/jira/cloud_client.go apps/backend/internal/jira/adf_description_test.go
timeout --kill-after=10s 330s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB GOFLAGS='-trimpath -p=1 -tags=fts5' bash scripts/lint-go-changed apps/backend/internal/jira/cloud_client.go apps/backend/internal/jira/adf_description_test.go

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/integrations/requirements/jira-watcher-task-prompts.md',
  'docs/specs/integrations/system-design/jira-watcher-task-prompts.md',
  'docs/plans/jira-nested-description-blocks/plan.md',
  'docs/plans/jira-nested-description-blocks/task-01-preserve-nested-adf-paragraphs.md',
];
const changedFiles = [
  { filename: 'apps/backend/internal/jira/cloud_client.go', status: 'modified' },
  { filename: 'apps/backend/internal/jira/adf_description_test.go', status: 'added' },
  ...docs.slice(1).map(filename => ({ filename, status: 'modified' })),
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
NODE
git diff --check -- apps/backend/internal/jira docs/specs/integrations docs/plans/jira-nested-description-blocks
git diff --cached --check
git status --short -- docs/plans/jira-nested-description-blocks
```

The coverage preflight uses the real repository `validateCoverage` evaluator
against the complete proposed file set and current document contents. It
validates both the unstaged design package and later implementation references;
it does not require publication or synthetic Git commits. If actual scope
changes, update the set rather than reporting stale coverage.

Retain every native command handle and join to actual exit. A timeout is not a
test verdict. Cleanup applies only to current owned command descendants and
test HTTP servers. No full-backend lint, unaffected suites, validator unit
tests, cache wipes, or foreign process cleanup are part of these checks.

Public docs have no owned edit. If a material discrepancy makes them affected,
record the exact path and run the real `node scripts/validate-public-docs.mjs`
validator before delivery; unchanged validator unit tests remain excluded.

## Owned files

- Production: `apps/backend/internal/jira/cloud_client.go` only.
- New tests: `apps/backend/internal/jira/adf_description_test.go` only.
- Reconciled design: `docs/specs/integrations/system-design/jira-watcher-task-prompts.md`.
- Delivery records: `docs/plans/jira-nested-description-blocks/plan.md` and
  `docs/plans/jira-nested-description-blocks/task-01-preserve-nested-adf-paragraphs.md`.
- Contract input, unchanged: `docs/specs/integrations/requirements/jira-watcher-task-prompts.md`.

## Dependencies

None. The existing shared converter and prior watcher-description fetch fix
are present in the qualification baseline.

## Risks

Delimiter duplication, accidental newline normalization, boundary loss around
empty/malformed children, and missing real-client coverage are the relevant
compatibility risks. The regression matrix constrains each one.

## Parallelism

`sequential`

## Inputs

- Owning requirement `.2`, `.3`, `.4` and design Description conversion section.
- Existing `issueToTicket`, `extractDescription`, `walkADF`, and REST tests.
- ROOT's qualified HTTP proof and protected private source ownership.
- Atlassian [listItem](https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/listItem/)
  and [paragraph](https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/paragraph/) schemas.

## Results

Implemented the shared converter correction and the permanent regression matrix
after the explicit implementation release. Structural paragraph separators are
deferred until readable content follows; an existing document sibling separator
satisfies the pending boundary. The converter still preserves literal and
explicit newline contributions and skips malformed/empty nodes tolerantly.

Actual local results for the exact verification commands above:

- Ticket HTTP RED: native handle 64551 joined exit 1, package 0.082s; exact
  fused-text assertion failed before production changed.
- Watcher HTTP RED: native handle 94483 joined exit 1, package 0.082s; the
  independent fixture produced the same causal assertion failure.
- Baseline compatibility: native handle 61986 joined exit 0, package 1.084s.
- Focused GREEN: native handle 74042 joined exit 0, package 1.087s.
- Complete affected Jira race package: native handle 92356 joined exit 0,
  package 1.335s.
- Staged affected-package lint: native handle 27485 joined exit 0, `0 issues.`
- `gofmt` completed with no remaining formatting differences.
- `python3 scripts/list-docs.py validate`: exit 0.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- Real `validateCoverage` preflight: exit 0, `covered`, `errors: []`.
- Scoped working/staged diff checks and plan status inspection: passed.

The frozen install joined exit 0 once, using pnpm 9.15.9; Go 1.26.0 and Node
24.18.0 were existing installations. Public docs remain unaffected for the
reason stated in the plan. No broad backend/browser suite or unchanged validator
unit suite was run. Publication, hosted reviews/checks, and any later merge
evidence remain in the external task plan.
