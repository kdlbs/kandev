---
id: "01-isolate-secret-list-lifetimes"
title: "Isolate secret list lifetimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.14
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Isolate secret list lifetimes

## Exact source and test scope

- `apps/web/hooks/domains/settings/use-secrets.ts` (only production edit).
- `apps/web/hooks/domains/settings/use-secrets.lifetime.test.tsx` (new).
- `apps/web/components/settings/secrets-settings.lifetime.test.tsx` (new).
- Four delivery artifacts: owning requirement/design and this plan/work order.
- Existing `use-secrets.test.ts` and `secrets-settings.test.ts` run as unchanged controls.

## Summary

Separate the existing hook's Global and Workspace effects so unrelated Global hydration
cannot reinitialize or cancel the Workspace metadata list. Cover the real subscription
boundary and actual rendered settings mutation acknowledgment path in one TDD pass.

## In scope

- The plan's full test matrix, including stable supplied/no initial lists, pending/settled
  Workspace reads against Global pending/success/failure, acknowledged add/update/remove,
  ordinary Global sharing/filtering, intentional scope/initial-list changes, and cleanup.
- Real `StateProvider`/`createAppStore`/`useSecrets` and rendered `SecretsSettings`, including
  real shared Save, delete preflight/confirmation, and acknowledgments; transport mocks only.
- Effect split preserving local state, committed scoped-key guard, initial-list reference
  lifecycle, Global shared loaded/loading behavior, and existing error settlement.

## Out of scope

Values/reveal/access/authorization/backend/API/profile policy; generic caches, cross-instance
Workspace sharing, same-list read/mutation races, stale-navigation mutation callbacks,
error/retry redesign, UI/copy/touch/navigation/browser/build/E2E changes, test-only production
modules/callbacks, framework/dependency/lockfile changes, sibling task files, and harness work.

## Acceptance

1. The regression `preserves acknowledged workspace mutations across global loading and
   settlement` fails against the unchanged producer for the archived metadata-loss reason,
   then passes with the effect split. Pending scoped reads retain one call and their signal
   during unrelated Global changes; actual rendered settings acknowledgments survive them.
2. Every compatibility and cleanup case in the plan's matrix passes using the real store and
   hook with only transport mocked, including .1/.2/.4 controls. No test-only production
   interfaces or consumer edits are introduced.
3. All exact checks below pass with honest joined receipts; update work-order results and
   plan status, and confirm implementation matches the amended active/current owning pair. Publication
   and actual merge follow ROOT's standing delivery gates recorded in the Kandev task plan.

## Implementation sequence

After ROOT's later explicit same-session implementation release, mark this work order
`in_progress`. Obtain the current global local-heavy lease before installation or any web
test/lint/typecheck/i18n command, including normal commit hooks. Read `/tdd` and scoped
frontend guidance. Write the two test files first, keeping transport mocks local and using
real existing providers/widgets. Record red evidence against the unchanged hook; then split
the effects and run the targeted suite and listed checks. Avoid repeat broad passing runs.

`initialItems` stays a referential replacement input; `loadedScopedKey` stays the committed
identity guard. Global reads retain their existing sharing gate and failure settlement.
Workspace cleanup still cancels late publication and aborts on relevant change/unmount.

### First RED execution boundary

The two permanent regression suites were authored before the heavy lease. The command
below then ran against the unchanged production hook under ROOT's lease, after the one
conditional frozen install:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-secrets.lifetime.test.tsx components/settings/secrets-settings.lifetime.test.tsx)
```

Record the actual RED result before any production edit. Import, provider, or collection
errors are not behavioral RED; correct only the owned test harness if needed and retain
ROOT's resource/timeout/checkpoint rules. Corrected behavioral RED was recorded before the effect split.

## Verification

These task-defined commands ran after implementation release under ROOT's heavy lease.
Run from repository root using Bash with `login=false`, the existing Node24 PATH, and a
4 GiB Node cap. Conditional frozen install runs at most once if dependencies are absent;
do not use ambient pnpm12, alter the lockfile, or install in the design turn.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
if [ ! -d apps/node_modules ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-secrets.lifetime.test.tsx components/settings/secrets-settings.lifetime.test.tsx hooks/domains/settings/use-secrets.test.ts components/settings/secrets-settings.test.ts)
(cd apps/web && corepack pnpm@9.15.9 exec eslint --max-warnings=0 hooks/domains/settings/use-secrets.ts hooks/domains/settings/use-secrets.lifetime.test.tsx components/settings/secrets-settings.lifetime.test.tsx)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet --base 3328fe887f0e9ea2ffb11a00c4c5d0a94a7b88ed)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Retain every command handle and ACTUALLYJOIN it before returning the lease. Record command,
exit, test counts, duration, and receipt paths. Timeout/resource/transport failures are not
PASS; checkpoint ROOT without automatic retry, cache wiping, foreign process termination,
or broader local work. Typecheck's existing pretypecheck generates release/changelog data;
unexpected tracked changes are a checkpoint, not authorization to publish extra files.
Reacquire the heavy lease for a valid fixup. No lease is held at the design checkpoint.

### Documentation reference coverage preflight

Run this lightweight preflight from repository root in design and again before publication.
It uses actual documents and the projected production/test scope rather than GitHub calls.
After implementation, additionally feed the actual changed/untracked file inventory to
`validateCoverage`; compare it to the owned scope and report drift before publication.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/workspaces/requirements/repository-secrets.md',
  'docs/specs/workspaces/system-design/repository-secrets.md',
  'docs/plans/workspace-secret-list-isolation/plan.md',
  'docs/plans/workspace-secret-list-isolation/task-01-isolate-secret-list-lifetimes.md',
];
const source = [
  'apps/web/hooks/domains/settings/use-secrets.ts',
  'apps/web/hooks/domains/settings/use-secrets.lifetime.test.tsx',
  'apps/web/components/settings/secrets-settings.lifetime.test.tsx',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...documents, ...source].map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.workOrders.length !== 1 ||
    result.acceptedReferences.length !== 1 ||
    result.acceptedReferences[0].designPath !== documents[1] ||
    !result.acceptedReferences[0].requirements.includes('REQ-WORKSPACES-REPOSITORY-SECRETS-001')) {
  process.exit(1);
}
NODE
```

## Files likely touched

Only the exact source/test and four delivery artifacts listed above. Read-only inputs include
`components/settings/secrets-settings.tsx`, real state/save/toast providers, settings-slice,
the supplied-list page, active SPA settings routes, and unchanged compatibility suites.

## Dependencies

None. ROOT's explicit implementation release, current local-heavy lease, and later separate
merge lease are execution gates, not work-order dependencies.

## Risks

See the plan's risks. A component harness that mocks settings widgets or providers does not
supply the required acknowledgment proof. Distinguish stable initial array references from
intentional replacements and unrelated Global changes from same-list concurrency.

## Parallelism

`sequential`. Existing primary session only; no delegates/new tasks/sessions/tabs/model changes.

## Inputs

- [Owning requirement](../../specs/workspaces/requirements/repository-secrets.md), .1/.2/.4/.14.
- [Owning design](../../specs/workspaces/system-design/repository-secrets.md), Metadata list lifetimes.
- [Plan](plan.md), source inventory, archived red evidence, and full test matrix.
- Existing real-provider pattern: `hooks/domains/settings/use-automations.test.tsx` (transport
  mocking/provider composition only, without importing its workspace cache design).
- Existing rendered shared-save pattern: `components/settings/settings-save-provider.test.tsx`.

## Results

Local implementation complete on 2026-10-05. The single production edit separates the
Global and Workspace effects; no consumer, transport contract, value, authorization,
profile binding, or UI behavior was changed beyond metadata persistence.

Corrected permanent RED: 28 expected behavioral failures and eight compatibility passes
across 36 cases, exit 1 (12.71s), before the production edit. The first run contained both
behavioral failures and a test setup-return error; the owned callback was corrected and
RED rerun without fixture/import errors. Full affected GREEN passed all 47 cases in four
files (19.35s). Formatting rerun passed all 36 new cases (12.46s); a lint-only test-block
split was followed by all 24 hook cases passing (3.02s).

Changed-file ESLint with zero warnings, typecheck, i18n check, base-pinned ratchet, catalog,
36 specification-linter tests, full specification lint, actual seven-file reference coverage,
and whitespace checks passed. One frozen pnpm 9.15.9 install completed; no lockfile or
tracked generated-file drift. Every command handle was actually joined. Detailed handles,
logs, normal hook receipt and hosted delivery evidence are preserved in the Kandev task plan.

The owning requirement/design remain active/current. The completed plan/work order record
local implementation; hosted required gates, substantive current-head review, ROOT's merge
lease and independently verified actual merge remain external completion gates.
