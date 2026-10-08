---
id: "01-system-query-owner-guard"
title: "Guard migrated System Query ownership"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001
acceptance_criteria:
  - AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.1
  - AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.2
  - AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.3
  - AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.4
system_design:
  - ../../specs/architecture-lint/system-design/migrated-system-query-owner.md
---

# Task 01: Guard migrated System Query ownership

## Outcome

Prevent the four accepted System Query snapshots from regaining a supported
second Zustand owner. Emit an actionable resource, location, and Query-hook
diagnostic. Keep the unrelated System owners and all application runtime
behavior unchanged.

## Scope

- Add ESLint rule ID system-query-owner/no-migrated-system-zustand-owner using
  the installed typescript-eslint AST.
- Add RuleTester fixtures for supported owner forms and permitted lookalikes.
- Register the rule only for system-slice.ts, types.ts, index.ts, and
  hydration/hydrator.ts.
- Add ESLint API wiring tests that resolve the real config, verify owner
  severity, verify Query-hook exclusion, lint the current production paths,
  and use `lintText` positive fixtures with each exact production owner path.
  Assert rule ID, severity, line, resource, and Query replacement. The wiring
  assertions must fail if registration is removed or disabled for any path.
- Reconcile only LINT-02 in the architecture roadmap, the bounded frontend
  architecture-lint guidance, the old Python architecture-lint ADR, and
  apps/web/AGENTS.md. Add the scoped architecture-lint requirement, design,
  and accepted ADR referenced by this work order.

## Retired forms and replacements

| Resource | Owner forms to reject | Direct hydration forms | Replacement |
| --- | --- | --- | --- |
| About SystemInfo | system.info; default info: null; setSystemInfo | draft.system.info = value; deepMerge(draft.system, { info: value }) | useSystemInfo |
| Database statistics | system.database; default database: null; setSystemDatabase | draft.system.database = value; deepMerge(draft.system, { database: value }) | useDatabaseStats |
| Backup list | system.backups; default { items: [], loaded: false }; setSystemBackups; SystemBackupsState alias and named barrel export | draft.system.backups = value; deepMerge(draft.system, { backups: value }) | useBackups |
| Disk usage | system.diskUsage; default diskUsage: null; setSystemDiskUsage | draft.system.diskUsage = value; deepMerge(draft.system, { diskUsage: value }) | useDiskUsage |

Accept identifier, quoted string, and computed static string-literal keys at
the positions defined in the system design. Recognize actual AST owners and
lexical binding identity: `SystemSliceState.system`, `SystemSliceActions`, the
`defaultSystemState.system` initializer, the object returned by
`createSystemSlice`, its `set` recipe parameter, the exact named barrel export,
and the first parameter of `hydrateState`. Ignore shadowed `draft` or `system`
names, nested callbacks, and local unrelated objects. The precise traversal
and supported forms are specified in the [system design](../../specs/architecture-lint/system-design/migrated-system-query-owner.md).
For slice mutations, resolve `set` to `createSystemSlice`'s first parameter
binding and `draft` to the direct recipe callback's first parameter binding.
For hydration, resolve `draft` to `hydrateState`'s first parameter binding;
`deepMerge` must resolve to the named production import from
`./merge-strategies`, with the exact authoritative `draft.system` as argument
one and a direct object literal as argument two. Same-spelled shadowed or
nested bindings, unrelated local helpers, and nested callback writes are not
owner shapes. Do not resolve arbitrary aliases or perform data-flow analysis.

Keep jobs, metrics, updates, retention, storage policy, overview,
analysisRevision, runs, quarantine, and especially `storage.disk` valid. Allow
the resource types, legitimate Query calls and hooks, fetch usage, independent
control-plane no-store process reads, comments, strings, templates, test
fixtures, unrelated domains, and same-named properties below another object.
Do not add a baseline, exemption, or broad suppression.

## Likely files

- apps/web/eslint-rules/no-migrated-system-query-owner.mjs
- apps/web/eslint-rules/no-migrated-system-query-owner.test.ts
- apps/web/eslint.config.mjs
- apps/web/scripts/lib/migrated-system-query-owner-wiring.test.ts
- apps/web/lib/state/slices/system/system-slice.ts
- apps/web/lib/state/slices/system/types.ts
- apps/web/lib/state/slices/system/index.ts
- apps/web/lib/state/hydration/hydrator.ts
- docs/specs/architecture-lint/requirements/migrated-system-query-owner.md
- docs/specs/architecture-lint/system-design/migrated-system-query-owner.md
- docs/specs/architecture-lint/README.md
- docs/decisions/2026-10-08-bounded-frontend-eslint-architecture-guard.md
- docs/decisions/2026-08-01-architecture-lint-budgets.md
- docs/architecture-lint.md
- docs/architecture-maintenance/lint-roadmap.md (LINT-02 row only)
- apps/web/AGENTS.md
- docs/plans/migrated-system-query-owner/plan.md
- docs/plans/migrated-system-query-owner/task-01-system-query-owner-guard.md

No production state, API, UI, boot-payload, backend, dependency, lockfile,
runtime flag, Python linter, baseline, or broad pre-commit hook changes are in
scope.

## Verification

Install dependencies in a fresh worktree first:

~~~bash
(cd apps && pnpm install --frozen-lockfile)
~~~

Run the focused rule and real-config wiring suites (each command starts from
its stated directory):

~~~bash
(cd apps && pnpm --filter @kandev/web test -- --run \
  eslint-rules/no-migrated-system-query-owner.test.ts \
  scripts/lib/migrated-system-query-owner-wiring.test.ts)
~~~

Lint the guarded production boundaries, rule, tests, and config directly:

~~~bash
(cd apps && pnpm --filter @kandev/web exec eslint --max-warnings 0 \
  lib/state/slices/system/system-slice.ts \
  lib/state/slices/system/types.ts \
  lib/state/slices/system/index.ts \
  lib/state/hydration/hydrator.ts \
  eslint-rules/no-migrated-system-query-owner.mjs \
  eslint-rules/no-migrated-system-query-owner.test.ts \
  scripts/lib/migrated-system-query-owner-wiring.test.ts \
  eslint.config.mjs)
~~~

Run the same frontend gates used by the `Frontend Tests Passed` CI job. These
are separate subshells so each command resolves paths from the correct root:

~~~bash
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run test)
~~~

For production-boundary red/green evidence, temporarily insert the retired
field into the actual defaultSystemState.system object in system-slice.ts and
run the targeted ESLint command. Confirm the named resource and Query hook in
the failure, restore the exact original file contents, then confirm it passes.
The wiring suite must use the real ESLint config and call `lintText` with each
exact production owner filename: `types.ts`, `system-slice.ts`, `index.ts`, and
`hydrator.ts`. For each path, inject supported structure and assert rule ID,
error severity, source line, resource, and replacement hook. Verify resolved
error severity for all four owner paths and no registration for the four Query
hook paths. Test negative lookalikes for jobs, metrics, updates, retention,
`storage.disk`, other storage state, Query hooks/calls, unrelated domains,
nested unrelated object fields, shadowed `draft`/`system`/`set`/`deepMerge`
bindings, nested callbacks, comments, strings, and templates. The tests must
fail when the rule is removed or disabled, or any one owner path is dropped
from config.

For the real-source red/green, use a narrow patch to the actual
`defaultSystemState.system` object in `system-slice.ts`, retain the original
file text in the shell/work notes, run targeted ESLint and capture the
resource-specific error, then restore only that temporary edit and rerun the
same command successfully. Do not use `git checkout`, `git restore`, or reset
commands that could discard concurrent work.
Record the exact red and green output without retaining an exemption.

The existing pre-commit web-lint hook matches only changed .ts, .tsx, .js, and
.jsx paths. Verify its real owner-file path with:

~~~bash
pre-commit run web-lint --files \
  apps/web/lib/state/slices/system/system-slice.ts \
  apps/web/lib/state/slices/system/types.ts \
  apps/web/lib/state/slices/system/index.ts \
  apps/web/lib/state/hydration/hydrator.ts
~~~

The same hook does not select the .mjs rule or eslint.config.mjs paths:

~~~bash
pre-commit run web-lint --files \
  apps/web/eslint-rules/no-migrated-system-query-owner.mjs \
  apps/web/eslint.config.mjs
~~~

That invocation is skipped by the hook's file filter; prettier-format does
select .mjs. The normal frontend CI full lint and web test gates cover the
rule/config edits. Do not expand hooks for this bounded rule.

Run documentation and change hygiene checks independently from the repository
root:

~~~bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
~~~

For local documentation-coverage preflight, first confirm that the local
validator is the trusted-base implementation:

~~~bash
git hash-object .github/scripts/pr-docs.cjs
git rev-parse origin/main:.github/scripts/pr-docs.cjs
~~~

Then call only its pure `validateCoverage` export with the actual pull-request
change set from the merge base and current package contents. Include untracked
files while the change is still local. This does not run the workflow adapter or
publish an external status:

~~~bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const trusted = execFileSync('git', ['rev-parse', 'origin/main:.github/scripts/pr-docs.cjs'], { encoding: 'utf8' }).trim();
const local = execFileSync('git', ['hash-object', '.github/scripts/pr-docs.cjs'], { encoding: 'utf8' }).trim();
if (local !== trusted) throw new Error(`coverage validator differs from origin/main: ${local} != ${trusted}`);
const mergeBase = execFileSync('git', ['merge-base', 'HEAD', 'origin/main'], { encoding: 'utf8' }).trim();
const list = args => execFileSync('git', args, { encoding: 'utf8' }).trim().split('\n').filter(Boolean);
const tracked = list(['diff', '--name-only', mergeBase]);
const untracked = list(['ls-files', '--others', '--exclude-standard']);
const paths = [...new Set([...tracked, ...untracked])].sort();
const changedFiles = paths.map(filename => ({ filename, status: untracked.includes(filename) ? 'added' : 'modified' }));
const docs = paths.filter(path => path.endsWith('.md') && fs.existsSync(path));
const fileContents = Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  mergeBase,
  trustedValidator: trusted,
  changedFiles: paths,
  status: result.status,
  acceptedReferences: result.acceptedReferences,
  errors: result.errors,
}, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
~~~

Record the trusted script blob and result. The required PR documentation
coverage gate remains authoritative and must pass on the final changed-file
set.

Measure full web lint wall time in three runs before implementation and after
registration with the same Python runner and checkout. The pre-registration
baseline is 55.14, 52.83, and 72.24 seconds (median 55.14 seconds); report the
three comparable post-registration runs and median:

~~~bash
python3 - <<'PY'
import subprocess
import time
for run in range(1, 4):
    started = time.perf_counter()
    completed = subprocess.run(["pnpm", "--filter", "@kandev/web", "lint"], cwd="apps")
    elapsed = time.perf_counter() - started
    print(f"web-lint run {run} seconds={elapsed:.2f}", flush=True)
    if completed.returncode:
        raise SystemExit(completed.returncode)
PY
~~~

Report each run and both medians. If practical, separately record the focused
RuleTester/config-wiring suite runtime. The rule is limited to four owner ASTs
and adds no whole-tree text scan. Dynamic/template computed keys, indirect or
local type aliases, spreads, helper indirection, generic merge payloads,
renamed semantic owners, and files outside those owner paths remain documented
limitations.

## Dependencies

None. The four Query migrations are already merged and their ownership designs
are the source contracts.

## Design-checkpoint evidence

- The trusted-base architecture-lint spec catalog contained the architecture
  system README and only the deprecation-ledger requirement/design pair. It had
  no existing frontend ownership-enforcement requirement, so this package adds
  one internal tooling contract rather than repurposing the deprecation
  ledger's acceptance criteria.
- The specification catalog validated 366 decisions and 1,475 specifications;
  the full specification-structure lint passed, and the new ADR is discoverable.
- The trusted-base coverage preflight matched local validator blob
  4569a63bf3da88bc31b1d6b34d7f03bf20269e93 to
  origin/main:.github/scripts/pr-docs.cjs. Evaluating the intended rule,
  config, four owner paths, and package artifacts returned covered, one
  accepted requirement/design reference, and no errors. No workflow adapter
  ran and no external status was published. The actual PR gate must be rerun
  against the final changed-file set.
- The actual pre-commit web-lint invocation passed for the four guarded
  TypeScript owner paths. The invocation on only the .mjs rule and
  eslint.config.mjs was skipped as expected by the hook file filter.
- Pre-registration full web-lint runs were 55.14, 52.83, and 72.24 seconds;
  the median was 55.14 seconds. Post-registration runs were 67.32, 64.70, and
  106.52 seconds, with a 67.32-second median. Variance is high; these results do
  not establish a rule-specific performance cost or a performance improvement.

## Results

The implementation adds the scoped ESLint rule, RuleTester matrix, real-config
wiring tests, and registration. Red/Green evidence: before implementation, the
real-config wiring suite had 8 failing assertions across the four exact owner
paths; the empty rule then failed 35 of 42 supported RuleTester cases. A
temporary `info: null` field in the real `defaultSystemState.system` object
failed ESLint with `About SystemInfo is Query-owned; read it with useSystemInfo`.
The temporary edit was restored byte-for-byte, and targeted ESLint passed.

Final focused RuleTester and wiring suites passed: 2 files, 64 tests, 4.27
seconds. Targeted ESLint passed on the four production owner files, rule,
tests, and configuration. Full web lint passed in 67.32, 64.70, and 106.52
seconds (median 67.32); typecheck passed in 82.7 seconds. Full web tests passed
with 2,758 files and 24,613 tests passed, 4 skipped, in 3,201.21 seconds. Some
integration child processes logged `ECONNREFUSED localhost:3000`; Vitest exited
successfully with no failed tests.

The final scoped pre-commit run passed, including documentation catalog and
specification lint, formatting, changed-file web lint, i18n, and public-copy
checks. The trusted-base coverage preflight used merge base
6254b05eb0242b67900e160ff5b1d9acbb7962ad and validator blob
4569a63bf3da88bc31b1d6b34d7f03bf20269e93 (matching `origin/main`); the actual
14-file scoped diff was covered by the new requirement/design with one accepted
reference and no errors. It invoked only `validateCoverage`; the required PR
coverage gate remains pending. The rule has zero owner findings or exemptions.

No application runtime, cache, hydration, API, UI, or backend source change is
retained. The guard does not resolve aliases or arbitrary data flow, dynamic or
template keys, object spreads, indirect helper payloads, renamed owners, or
mirrors outside the four configured source files. The tested full-lint time
range was 64.70–106.52 seconds after registration versus 52.83–72.24 seconds
before; run-to-run variance prevents attributing the difference to this rule.
