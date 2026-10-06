---
id: "01-validate-preview-ports"
title: "Validate complete preview ports"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PREVIEW-URL-DETECTION-001
acceptance_criteria:
  - AC-UI-PREVIEW-URL-DETECTION-001.1
  - AC-UI-PREVIEW-URL-DETECTION-001.2
  - AC-UI-PREVIEW-URL-DETECTION-001.3
  - AC-UI-PREVIEW-URL-DETECTION-001.4
  - AC-UI-PREVIEW-URL-DETECTION-001.5
system_design:
  - ../../specs/ui/system-design/preview-url-detection.md
---

# Task 01: Validate complete preview ports

## Summary

Prevent numeric overflow and truncated bare port tokens from becoming preview
candidates. Preserve valid-candidate selection and prove the real exported
output-to-proxy rewrite pipeline retains a working preview.

## In scope

- Add permanent regressions to the existing detector test file before editing
  production code. Use the test matrix in [the plan](plan.md#tests).
- Correct complete-token matching and range validation in the existing detector.
- Update this work order and plan with actual command results; after all checks
  pass and implementation conforms, promote paired requirements to `active`,
  paired design to `current`, this work order to `done`, and plan to `implemented`.

## Out of scope

Backend/proxy admission, caller changes, API/schema/dependencies, runtime
lifetime, coordinators, framework changes, layout/copy, new files for tests or
code, and general URL parsing cleanup. No production or permanent test edits
until a later explicit ROOT reviewed-package implementation INTERRUPT to this
same session. Preserve paused oversized task and its worktree; no delegates.

## Acceptance

1. Targeted permanent tests fail for the supplied overflow/truncation and
   selection defects before production changes, then pass with the minimum
   correction. Invalid full and bare numeric tokens never produce a truncated
   candidate; all supported hosts and full/bare upper boundary are covered.
2. Mixed same-line and multi-line cases retain first-valid-full,
   last-valid-bare-fallback, and last-valid-line behavior. Real exported output
   detection followed by real rewrite retains the exact proxy URL after invalid
   later output and rewrites a valid65535 candidate; only config is mocked.
3. Existing lower boundary, bare width, host/scheme/path/query/hash/ANSI behavior,
   signatures, and consumers remain compatible. All required targeted checks
   and documentation coverage pass within owned files; mobile parity uses the
   explicit pure-data exception in the paired design.

## Verification

Run only after explicit release. Use Node24 and project-pinned pnpm9.15.9.
Commands below run from repo root and root package operations independently.
Run heavy commands sequentially, retain each session handle, and join it before
starting another. On resource failure checkpoint for ROOT; do not auto-repeat.

This session's shell omits Node from PATH; the existing Node24 install also
contains corepack. Expose it before the later commands, without an install:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
```

Conditional dependency preparation, once only if dependencies are missing:

```bash
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

RED after adding regressions, then GREEN after production correction. These are
the same command at distinct TDD stages; do not rerun passing tests without a
required correction:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run lib/preview-url-detector.test.ts --maxWorkers=1)
```

After GREEN, run each command once unless a required correction justifies an
affected rerun:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 lib/preview-url-detector.ts lib/preview-url-detector.test.ts)
(cd apps && corepack pnpm@9.15.9 exec prettier --check web/lib/preview-url-detector.ts web/lib/preview-url-detector.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Use the repository's real documentation validator for actual changed files,
including untracked docs, before committing. This local evaluation does not
contact GitHub or stage files:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = 'f18fe2d9e3bafb567aff643f25356ac3ed651120';
// Native subprocess output bypasses display helpers; RTK is unavailable here.
const rawGit = args => execFileSync('git', args, { encoding: 'utf8' });
const paths = [...new Set([
  ...rawGit(['diff', '--name-only', '-z', base]).split('\0'),
  ...rawGit(['ls-files', '--others', '--exclude-standard', '-z']).split('\0'),
].filter(Boolean))];
const docs = [
  'docs/specs/ui/requirements/preview-url-detection.md',
  'docs/specs/ui/system-design/preview-url-detection.md',
  'docs/plans/reject-invalid-preview-ports/plan.md',
  'docs/plans/reject-invalid-preview-ports/task-01-validate-preview-ports.md',
];
const allowed = new Set([...docs,
  'apps/web/lib/preview-url-detector.ts',
  'apps/web/lib/preview-url-detector.test.ts',
]);
if (paths.some(p => !allowed.has(p))) throw new Error('Changed files exceed work-order scope');
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles: paths, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

Keep normal active commit hooks enabled. No Go/build/app runtime/E2E, broad
audit, or ROOT-proof replay. The mobile exception covers data normalization
only; any required structural UI change must checkpoint rather than expand.

## Files likely touched

- `apps/web/lib/preview-url-detector.ts`
- `apps/web/lib/preview-url-detector.test.ts`
- `docs/specs/ui/requirements/preview-url-detection.md`
- `docs/specs/ui/system-design/preview-url-detection.md`
- `docs/plans/reject-invalid-preview-ports/plan.md`
- This work order.

## Dependencies

None. Later explicit ROOT release is an execution barrier, not implicit
authorization from this pending work order.

## Risks

See [the plan](plan.md#risks). Numeric token matching must not backtrack into an
invalid prefix. Full-match preference must not be changed by reverse bare scanning.

## Parallelism

`sequential`. Preserve this primary agent/executor and session; no delegates.

## Inputs

- [Requirements](../../specs/ui/requirements/preview-url-detection.md).
- [Design](../../specs/ui/system-design/preview-url-detection.md).
- Existing detector/test file and scoped `apps/web/AGENTS.md`.
- Accepted ROOT proof and identity/delivery gates in [the plan](plan.md).

## Results

Completed on 2026-10-05 after ROOT's explicit implementation release.

- One conditional `corepack pnpm@9.15.9 install --frozen-lockfile` from `apps/`
  completed, exit 0; 923 packages reused, none downloaded.
- Targeted Vitest command above: RED exit 1 with 37 expected failures and 62
  passes. GREEN 99/99 pass, exit 0. After a required duplicated-test-literal lint
  correction, final targeted rerun passed 99/99, exit 0.
- Scoped ESLint command above: initial exit 1 for one duplicated test literal;
  corrected with a test constant, final exit 0 with no warnings.
- Changed-file Prettier check: exit 0. Formatter touched only the owned test file.
- Web `typecheck`: exit 0, including normal pretypecheck generation.
- `i18n:check`: exit 0; existing 429 unreferenced catalog entries are warnings.
- `i18n:ratchet`: exit 0; modified production file and guard allowlist clean.
- `python3 scripts/list-docs.py validate`: exit 0, 349 decisions/1345 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `git diff --check`: exit 0; status contains exactly the six owned paths.
- Actual changed-file Node documentation preflight above: exit 0, `covered`,
  zero errors, all six actual paths within scope and linked package accepted.

All command handles were joined. The complete ROOT proof was not replayed or
modified. No Go/build/E2E/app runtime or broad audit ran. Mobile parity uses the
reviewed pure-data exception; public docs need no change because this restores
preview candidate selection without introducing a user procedure or UI contract.

Implementation conforms to the paired requirement/design, promoted to
`active`/`current`. Normal hook receipts and exact-head hosted review, CI, merge,
and owned-cleanup results remain external delivery gates in the versioned live
plan; this work-order result does not claim a merge.
