---
id: "01-redact-share-metadata"
title: "Redact share metadata"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-PUBLIC-SHARE-LINKS-001
acceptance_criteria:
  - AC-AUTH-PUBLIC-SHARE-LINKS-001.6
  - AC-AUTH-PUBLIC-SHARE-LINKS-001.7
  - AC-AUTH-PUBLIC-SHARE-LINKS-001.8
system_design:
  - ../../specs/auth/system-design/public-share-links.md
---

# Task 01: Redact Share Metadata

## Summary

Complete metadata redaction at the existing snapshot-construction boundary.
Independently prove the preview and mock gist artifacts retain ordinary content
while removing values matched by the existing rules and recording those rules.

## In scope

- Own the minimal `builder.go` correction and new `metadata_redaction_test.go`
  regressions in `apps/backend/internal/task/share`.
- Use one existing redactor for task title, three session strings, and the
  existing message pipeline, before collecting applied rules.
- Run RED before production edits, then GREEN and the complete affected package.
- Update the existing public redaction paragraph with implemented coverage and
  the frozen historical-share limit.
- Record actual results and close this order and its manifest before publication.
- Correct only the proven fake-prober registration barrier that failed required
  backend CI, preserving runtime behavior and the one-dial/response assertions.

## Out of scope

- Protected ROOT candidate test, new patterns, generic JSON scrubbing,
  identifiers/timestamps/version changes, source mutation, or message redesign.
- Historical gist rewriting/scanning/revoking and live publication.
- Provider credentials/auth, schema, transport, UI/layout, runtime flags, or
  unrelated cleanup, tests, dependencies, caches, refs, processes, and worktrees.

## Acceptance

1. The independent regressions fail on raw metadata exposure before the fix and
   pass through the real builder, preview, and mock Gist upload after it. All
   three files and the description exclude the matched synthetic values, retain
   expected ordinary content, and the snapshot records the expected rule union.
2. The exported title/session strings use the same `Redactor.String` and roots
   as existing messages. Non-string/empty projection, workflow-step identifier,
   timestamps, schema/build identity, source models, and existing message/tool
   controls retain their behavior.
3. The targeted tests, complete affected package, staged package lint, docs
   validators, specification gates, and actual documentation coverage preflight
   pass. Results contain native joins and actual command outcomes.

## Verification

Design-only gate: do not run installation, product tests, or lint until a later
explicit implementation release. Hold the one global heavy-command lease for
implementation/install/checks, and run commands sequentially. If this fresh
worktree has no dependency installation yet, run once before hooks/package
commands, from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Mark the order `in_progress`, independently author the tests, then run RED
before changing production. Repeat the same targeted command for GREEN:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/task/share -run '^TestShareMetadataRedaction_' -count=1)
```

After GREEN, run the complete canonical affected package and the document gates
from the repository root. Go 1.26 is required by `apps/backend/go.mod`:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/task/share -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/public-share-metadata-redaction
```

Run the repository's actual offline PR-documentation coverage validator over
the owned changed paths and their four references. This reads workspace files
and does not publish a GitHub status:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/auth/requirements/public-share-links.md',
  'docs/specs/auth/system-design/public-share-links.md',
  'docs/plans/public-share-metadata-redaction/plan.md',
  'docs/plans/public-share-metadata-redaction/task-01-redact-share-metadata.md',
];
const changed = [...docs,
  'apps/backend/internal/task/share/builder.go',
  'apps/backend/internal/task/share/metadata_redaction_test.go',
  'docs/public/sessions-and-review.md',
];
const result = validateCoverage({
  changedFiles: changed.map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

After reviewed implementation checks, stage only the complete owned change,
format staged Go files and re-stage as needed, then run the package-scoped
canonical lint from the repository root before committing:

```bash
timeout --signal=TERM --kill-after=10s 360s env GOMAXPROCS=2 GOMEMLIMIT=512MiB bash scripts/lint-go-changed apps/backend/internal/task/share/builder.go apps/backend/internal/task/share/metadata_redaction_test.go
git diff --cached --check
```

For the required-CI fixture correction, after the bounded deterministic overlay
proof, run only the affected SSH test and complete package from the repository
root. Do not replay passing share/frontend/broad-backend checks:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/ssh -run '^TestProbeReachability_CoalescesConcurrentProbes$' -count=1)
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/ssh -count=1)
timeout --signal=TERM --kill-after=10s 360s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB bash scripts/lint-go-changed apps/backend/internal/ssh/reachability_handlers_test.go
```

Use the existing Go 1.26.0 bin directory at task PATH, stage the changed SSH
file before canonical lint, and preserve normal hooks. No SSH production change
or dependency update is in scope.

Retain and join every actual native command handle; do not infer completion
from a duplicate run. Preserve normal commit hooks and their required checks;
do not bypass them. The initial full-backend scan belongs to hosted CI. Release
the heavy lease after actual checks and publication work before hosted waiting.

## Files likely touched

- `apps/backend/internal/task/share/builder.go`
- `apps/backend/internal/task/share/metadata_redaction_test.go` (new)
- `apps/backend/internal/ssh/reachability_handlers_test.go` (required-CI fixture only)
- `docs/public/sessions-and-review.md` (one existing redaction paragraph)
- The owning requirement/design pair and this plan/order for final results.

## Dependencies

None. Implementation followed ROOT's later explicit release in this same
primary session after design review; the design handoff alone did not authorize it.

## Risks

The existing heuristics remain bounded and published gists remain frozen.
Regression fixtures must not accidentally trigger a message rule that masks a
metadata-only omission. Assertions must cover expected content and nonmutation,
not only absence. See the manifest's field audit and evidence limitations.

## Parallelism

`sequential`. No delegation, recursive task, extra session/tab, or model switch.

## Inputs

- [Requirement AC001.6 through AC001.8](../../specs/auth/requirements/public-share-links.md)
- [Snapshot metadata redaction and field audit](../../specs/auth/system-design/public-share-links.md#snapshot-metadata-redaction)
- [Manifest evidence, test matrix, and scope](plan.md)
- Existing builder, redactor, service preview, gist upload and test stubs in
  `apps/backend/internal/task/share`.
- `apps/backend/AGENTS.md`, `/tdd`, and its backend-test patterns.
- ROOT's qualified proof/log only; never its protected candidate source.

## Results

Done on 2026-10-13 before first publication. The independently authored tests
cover all four exported descriptive strings with each of the five existing
secret shapes and a synthetic path, through the real builder and all three mock
gist files plus description. Preview, metadata-only rule logs, multi-root
precedence, message/tool rule union, ordinary/empty/non-string values,
identifiers/version/timestamps, and source nonmutation controls pass.

- Frozen install: native 61007 joined exit 0, once, Node 24.18.0 and workspace
  pnpm 9.15.9; no lockfile changes.
- Targeted RED: native 68444 joined exit 1, package 0.098s. It compiled and
  reached metadata exposure/missing-rule assertions before production edits.
- Targeted GREEN: native 37874 joined exit 0, package 1.103s.
- Complete affected Go package: native 59350 joined exit 0, package 1.773s.
- Canonical staged whole-package lint: native 69649 joined exit 0, `0 issues.`
- Actual catalog validation (380 decisions/1580 specs), full specification lint,
  public-doc validator (48 pages), offline actual documentation coverage
  (`covered`, `errors: []`), and working/staged diff checks passed.

Go 1.26.0 came from the existing mise toolchain, prepended to task command PATH,
with `GOTOOLCHAIN=local` and the resource/timeout settings above. No validator
unit tests were repeated. The historical design receipt remains in the manifest.
The first formatting invocation had an incorrect cwd-relative path; it was
corrected before GREEN and was not a Go compilation failure. No share production edits
followed the passing product checks. No live gist publication was performed.

See the [manifest verification record](plan.md#verification-results) for full
local results. Commit-hook, push, exact-head hosted CI/review, and merge outcomes
remain external delivery evidence and are not represented as completed here.

Required-CI remediation completed before fixup publication: the actual failed
shard's two-dial assertion was reproduced deterministically by pausing the
second caller after the pre-registration counter. Witness RED native 85295
joined exit 1 (0.067s); after `DoChan` registration before signaling entry, the
same witness GREEN 87107 joined exit 0 (1.068s). Original one-dial/identical
response test 72361 joined exit 0 (1.066s), complete affected SSH package 43906
joined exit 0 (5.558s), and staged whole-package lint 39735 joined exit 0
(`0 issues.`). The original assertions and SSH runtime remain unchanged. Full
CI log/artifact, blob comparison, and temporary proof are retained externally.
See the manifest's required-CI correction record; hosted readiness remains
pending on the original observer and unchanged deadline.
