---
id: "01-preserve-current-metadata"
title: "Preserve current metadata on qualified Global server failure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-002
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.14
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.3
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.5
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.6
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Preserve current metadata on qualified Global server failure

## Summary and gate

Extend the existing Global initial-read fence to a qualified HTTP 5xx catch completion.
Prove the real form/save/API/event/store loss independently before the hook edit, then
verify whole-inventory retention and unchanged denial/error/Workspace policy.

Execution released after FULL ROOT actual-file design review and a LATER explicit
implementation interrupt to THIS SAME primary on 2026-10-12. Exclusive global local-heavy
lease is granted through checks and ordinary hooks/commit/push. The
[manifest](plan.md) and external task plan preserve identity, selected profile, system
marker, question barriers, title ownership, final-action rules, completion gates, and user
edits. Artifact creation/callbacks are not execution permission. No delegates, tasks,
sessions/tabs/model changes or additional worktrees.

## Inputs and owned files

- [Owning requirement](../../specs/workspaces/requirements/repository-secrets.md), 002.5/.6
  and existing success/compatibility ACs listed above.
- [Owning design](../../specs/workspaces/system-design/repository-secrets.md#failure-and-compatibility-boundary).
- [Manifest's exact planned test matrix](plan.md#tests-and-traceability) and supplied ROOT
  historical qualification, not private source. Never read/copy/delete ROOT fixture,
  scratch/proofs or replay ROOT's probe.
- Existing public actor/pattern files: `apps/web/components/settings/secrets-settings.global-publication.test.tsx`,
  `apps/web/hooks/domains/settings/use-secrets.global-publication.test.tsx`, real
  `components/state-provider.tsx`, `components/toast-provider.tsx`,
  `components/settings/settings-save-provider.tsx`, `components/settings/secrets-settings.tsx`,
  `lib/state/store.ts`, `lib/state/slices/settings/settings-slice.ts`,
  `lib/api/client.ts`, `lib/api/domains/secrets-api.ts`, `lib/ws/handlers/secrets.ts`
  (last paths relative to `apps/web`).

Production ownership: **only** `apps/web/hooks/domains/settings/use-secrets.ts`.

Independently author after release:

- `apps/web/components/settings/secrets-settings.initial-server-error.test.tsx`.
- `apps/web/components/settings/secrets-settings.initial-failure.test-helpers.tsx`.
- `apps/web/hooks/domains/settings/use-secrets.global-server-error.test.tsx`.

Delivery ownership: this order, sibling `plan.md`, and the same requirement/design pair.
Read-only providers/store/API/events/widgets stay unchanged. You are not alone in the
codebase: preserve others' edits and all foreign resources. Inventory complete actual
paths/status; expected files may compare, never filter out drift.

## In scope

- Primary mounted causal test and exactly the four independent controls named in the manifest.
- Mixed accepted create/edit/delete callbacks/events, all current metadata and exact array
  identity, accepted-empty/replacement inventory, two consumers/one read and readiness,
  Global/legacy filtering, 5xx range, actual 4xx/401 classification, nonqualified errors.
- Narrow typed error predicate and synchronous current-array settlement in Global catch.
- Exact affected tests, scoped type/lint, i18n, spec/coverage/whitespace and ordinary hooks.

## Out of scope

Authorization/error-policy expansion, successful fence changes, Workspace callbacks,
generic retry/refresh/lifecycle hardening, API/backend/reveal/encryption/binding/runtime,
plaintext exposure, state actions/cache/journal/version/framework/new production helper module,
dependencies/config/lockfile changes, markup/copy/controls/operator steps, browser/build,
full test/type/lint suites, and optional post-publication polish. Another error policy
requires a ROOT checkpoint before expansion; a parent-question call ends that turn.

## Acceptance

1. Independently authored `keeps accepted Global metadata and its row after older initial
   GET500` fails against unchanged production because the acknowledged metadata/row disappears,
   while uncontested GET200, loaded create/no GET, uncontested GET500, and accepted create/GET403
   controls pass. Real mounted providers/form/shared Save/API/registered event/store/rows
   are exercised with only external transport isolated, without setup/unhandled errors.
2. Only Global catch changes: actual `ApiError` status500–599 AND changed captured items
   reference retain the exact entire current array through `setSecrets`; otherwise existing
   clearing. Existing finally, loading/admission/success/filtering/Workspace policies persist.
   Mixed/two-consumer/error-range/auth tests pass with all membership/order/fields/readiness.
3. Every exact command below passes with actual counts/exits/original handles/joins and
   actual staged-helper ratchet evidence. Before first publication synchronize local statuses
   and closing notes. Hosted READY and normal expected-head merge remain separate ROOT gates.

## Implementation sequence

After review/release/lease read `/tdd`, scoped web guidance and this full package; mark the
order `in_progress`. Install frozen workspace dependencies once before pnpm commands.
Author independent tests/helper from public composition, without mocking the hook, store,
providers, API functions, registered handlers or widgets. Hold actual initial fetch; fill
real form with synthetic input, use shared Save, await POST200/store/DOM acceptance, and
dispatch `registerSecretsHandlers(store)` created echo. Assert exact row presence before
releasing GET500. Then assert expected retained array and row; this must be causal RED on
the unchanged hook. Verify each control passes. Record exit/duration/counts and actual join.

Implement only the reviewed catch/import. Use `error instanceof ApiError`, `status >= 500`
and `< 600`, plus current-versus-captured immutable array identity. Read/compare/set in
the callback with no intervening await/external call. Use existing `setSecrets` for loaded
settlement and unchanged `finally` for loading. Use one small unexported pure `failureItems` selector in this same file, with error,
captured initial items and synchronous current items as arguments. ROOT approved this
required-lint extraction after original GREEN; no helper module or state API.
Then run affected GREEN and task-defined checks. Stop on three focused failures or material
design conflict. Do not expand production scope to repair test setup.

## Verification

Commands run serially from repository root in Bash after release/lease only, with Node24
and pinned pnpm9.15.9. Explicit one worker/no file parallelism. No broad local suite.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism components/settings/secrets-settings.initial-server-error.test.tsx)
```

That is the first causal RED/four-control run on unchanged production. Retain the install
and RED receipts; do not reinstall during GREEN or infer completion from duplicate runs.
The original twelve-suite GREEN ran before the required-lint file-local extraction.
After that extraction ROOT requires only the two new failure suites again, plus scoped lint/type
and remaining checks; do not repeat twelve suites or reinstall. Original affected command:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism components/settings/secrets-settings.initial-server-error.test.tsx hooks/domains/settings/use-secrets.global-server-error.test.tsx components/settings/secrets-settings.global-publication.test.tsx hooks/domains/settings/use-secrets.global-publication.test.tsx components/settings/secrets-settings.workspace-publication.test.tsx hooks/domains/settings/use-secrets.workspace-publication.test.tsx components/settings/secrets-settings.lifetime.test.tsx hooks/domains/settings/use-secrets.lifetime.test.tsx hooks/domains/settings/use-secrets.test.ts components/settings/secrets-settings.test.ts lib/api/client.test.ts lib/api/domains/secrets-api.test.ts)
(cd apps/web && corepack pnpm@9.15.9 exec eslint --max-warnings=0 hooks/domains/settings/use-secrets.ts hooks/domains/settings/use-secrets.global-server-error.test.tsx components/settings/secrets-settings.initial-server-error.test.tsx components/settings/secrets-settings.initial-failure.test-helpers.tsx)
```

Required post-extraction GREEN:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism components/settings/secrets-settings.initial-server-error.test.tsx hooks/domains/settings/use-secrets.global-server-error.test.tsx)
```

Scoped TypeScript: create an owned external config extending the existing web tsconfig,
with only the hook/new tests/helper and existing Vite declaration as entrypoints; imported
application types remain real. It emits nothing and disables incremental writes. Record
the exact generated config path and contents; no permanent config or generated artifact.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
child114_types_dir=$(mktemp -d /tmp/kandev-child114-types-XXXXXX)
export CHILD114_TYPES_DIR="$child114_types_dir"
python3 - <<'PY'
import json, os
from pathlib import Path
root = Path.cwd()
files = [
    'hooks/domains/settings/use-secrets.ts',
    'hooks/domains/settings/use-secrets.global-server-error.test.tsx',
    'components/settings/secrets-settings.initial-server-error.test.tsx',
    'components/settings/secrets-settings.initial-failure.test-helpers.tsx',
    'vite-env.d.ts',
    'global.d.ts',
]
config = {
    'extends': str(root / 'apps/web/tsconfig.json'),
    'compilerOptions': {
        'incremental': False, 'noEmit': True, 'types': ['node'],
        'typeRoots': [str(root / 'apps/web/node_modules/@types'), str(root / 'apps/node_modules/@types')],
    },
    'include': [str(root / 'apps/web' / name) for name in files],
    'exclude': [],
}
(Path(os.environ['CHILD114_TYPES_DIR']) / 'tsconfig.json').write_text(json.dumps(config, indent=2) + '\n')
PY
(cd apps/web && corepack pnpm@9.15.9 exec tsc --noEmit -p "$CHILD114_TYPES_DIR/tsconfig.json")
```

Stage all actual owned package/source/test paths only after checks and status reconciliation.
Before staging verify the full raw inventory; preserve foreign edits. The final base-pinned
ratchet MUST run after the actual new helper is staged, not while it remains untracked.
Record scanner output and any test-helper exemption honestly; verify helper inclusion in
the staged added-file inventory rather than claiming an unstaged scan. No separate optional
ratchet later. Run ordinary active hooks on commit with no bypass.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
git add docs/specs/workspaces/requirements/repository-secrets.md docs/specs/workspaces/system-design/repository-secrets.md docs/plans/global-secret-initial-server-error/plan.md docs/plans/global-secret-initial-server-error/task-01-preserve-current-metadata.md apps/web/hooks/domains/settings/use-secrets.ts apps/web/hooks/domains/settings/use-secrets.global-server-error.test.tsx apps/web/components/settings/secrets-settings.initial-server-error.test.tsx apps/web/components/settings/secrets-settings.initial-failure.test-helpers.tsx
git diff --cached --name-only --diff-filter=A
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet --base d6887b98b60f530b7e52d5d874019ea1a960550a)
PYTHONDONTWRITEBYTECODE=1 python3 scripts/list-docs.py validate
PYTHONDONTWRITEBYTECODE=1 python3 scripts/lint-spec-files.test.py
PYTHONDONTWRITEBYTECODE=1 python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
git status --short --untracked-files=all
```

Run `.github/scripts/pr-docs.cjs` `validateCoverage` on the **complete actual** changed,
staged and untracked file inventory with actual document contents; production path must
be covered by this order, every AC must exist, referenced designs must declare the REQs
and appear in the manifest, and all local Markdown links must resolve. Record JSON/errors
externally. Design docs-only exemption does not prove production coverage. The design
checkpoint also runs a separate anticipated-production preflight with the existing real
hook path plus the four actual docs, honestly labelled as future-path coverage.

Every long command retains the FULL original exec result, yielded session_id and actual
join via the original handle. No overlapping heavy commands or discarded handles.
Record current absence of known-owned wrapper/groups after ALL local joins before lease
RETURN. No foreign kills/cache/worktree/ref/dependency removal. Preserve owned external
type config as a receipt; no cleanup is needed for retained evidence.

## Dependencies, mobile parity, and parallelism

No work-order dependencies. `sequential`; SAME primary only. Existing owner/source at actual
main is a reviewed input, not permission to drift-only rebase. The shared state/data-only
mobile exemption applies; no markup/copy/browser/build/ASCII requirements. Real mounted
workflow tests prove shared desktop/phone data semantics. Internal docs only; existing ADR
and public secret-scope instructions remain authoritative.

## Risks and later delivery

Overbroad status checks could preserve denied data. Checking a single row/nonempty list
misses whole-inventory semantics. Interleaved reads/setters or render snapshots lose
freshness. See the manifest's mixed-inventory and error controls.

Before FIRST publish, update this Results and manifest to actual local completion/status,
exits/joins and remaining hosted gates; stage those final notes within the original normal
commit. No optional docs-polish pushes later. Load commit/push/PR skills at authorized
delivery; conventional commits and active ordinary hooks apply. After all local joins,
normal commit/push and current owned-absence verification, concise lease RETURN then
DIRECT SAME-TURN hosted observation with sole original pr-await90m/cadence60/GNU91m/kill10.
Retain original observer/clock/handles through interrupts and actual fixes. No END/WFI,
duplicate observer, extra release ceremony or fresh FULL bot request per push/SHA.

Freeze published heads except actual code/data/security/required-CI fixes. Honor actual
approvals, use automatic feedback and substantive review plus bounded later diff; focused
rereview only for material correctness/security/architecture need. READY and separate
SERIAL normal expected-head squash are defined in the manifest. ROOT performs one static
compatibility check, grants merge separately, then independently verifies/archives once.
Verify actual merge/known-owned cleanup; preserve managed workspace/deps/media/cache.
Every ROOT instruction uses delivery_mode=interrupt; SAME primary/live handles retained.

## Results

Local implementation complete on 2026-10-12 after FULL ROOT review and LATER explicit
same-primary release under the exclusive global local-heavy lease. One production file:
`use-secrets.ts`, with typed ApiError import and one unexported pure selector approved by ROOT
for active function-length lint. Current array/reference selection and synchronous setter
preserve the reviewed 5xx boundary; success, 4xx/auth/nonqualified/Workspace policy unchanged.

Original independent causal RED31686 JOIN1: one missing-row failure/four passing controls,
five cases,11.86s. Original affected GREEN19494 JOIN0: twelve suites/110 tests,39.30s.
Required-lint extraction then final affected two-suite GREEN82796 JOIN0:21 tests,10.30s.
Lint63764 JOIN1 is retained; corrected final four-file lint88143 JOIN0 with zero warnings.
Type74772 JOIN2 is retained (test event-action widening and external ambient-input omission);
corrected scoped type19720 JOIN0. No application ambient/types/dependency change.

One frozen install25693 JOIN0; i18n14293 JOIN0; final actual STAGED helper ratchet14755 JOIN0
(one added plus one modified clean,643 guard entries). Spec checks45243 JOIN0:377 decisions,
1545 specs,36 validator tests,full lint. Complete inventory/coverage/whitespace exit0:
eight owned changed paths,111337 total entries,no post-install drift,13 links,10 ACs,
2 REQs,one order,production covered/errors[]. Full original receipts and external type config
are retained. All local command handles actually joined; no live local command remains.

Statuses and closing notes are synchronized before FIRST publish. Normal active hook commit,
push, hosted validation and separately granted normal expected-head squash remain delivery
gates, with receipts/final closure in the external task plan. No post-publication docs polish
or new FULL bot request is scheduled. No protected-source read/copy/delete or foreign cleanup.
