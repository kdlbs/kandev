---
status: current
system: architecture-lint
requirements:
  - REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001
created: 2026-10-08
owners:
  - kandev
---

# Migrated System Query Owner Guard

## Purpose and boundaries

This design adds one bounded frontend architecture check to the architecture-lint
system. It prevents supported direct Zustand mirrors of four existing Query
snapshots. It does not migrate another resource or change application runtime
behavior.

The accepted ownership contracts remain in the [SystemInfo Query
design](../../platform/system-design/system-info-query-cache.md),
[database-statistics design](../../system-page/system-design/database-statistics-snapshot.md),
[backup-list design](../../system-page/system-design/backup-list-query-cache.md),
and [disk-usage design](../../system-page/system-design/disk-usage-query-cache.md).
Their ownership decisions are
[SystemInfo](../../../decisions/2026-09-26-system-info-query-cache-ownership.md),
[database statistics](../../../decisions/2026-09-27-database-stats-query-cache-ownership.md),
[backup list](../../../decisions/2026-10-05-backup-list-query-ownership.md), and
[disk usage](../../../decisions/2026-10-05-disk-usage-query-invalidation.md).
The migrations were delivered by PRs #3977, #4225, #4271, and #4291 at
commits fad0a2f553, 059260b30f, d149627883, and 122f52018c.

The finite delivery packages are the [SystemInfo pilot](../../../plans/system-info-query-pilot/plan.md),
[database-statistics migration](../../../plans/database-statistics-query-migration/plan.md),
[backup-list migration](../../../plans/system-backup-query/plan.md), and
[disk-usage migration](../../../plans/disk-usage-query-migration/plan.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001 | Rule contract, Owner syntax and scope, Registration and verification |

## Current state composition

SystemSliceState.system defines the System fields. system-slice.ts owns
defaultSystemState.system and createSystemSlice. default-state.ts composes the
default System object, store.ts spreads createSystemSlice into the root
AppState, and app-state-types.ts derives HydrationState.system as a partial
AppState.system. hydrateState currently applies the generic
deepMerge(draft.system, state.system). It has no explicit mirror of the four
Query snapshots. This rule therefore checks the existing declaration/default
owners and only direct static writes or direct inline merge properties at the
existing hydration owner.

## Rule contract

Implement one custom ESLint rule using the repository's installed
typescript-eslint parser and existing ESLint RuleTester path:

- Rule ID: **system-query-owner/no-migrated-system-zustand-owner**
- Rule module: **apps/web/eslint-rules/no-migrated-system-query-owner.mjs**
- Rule tests: **apps/web/eslint-rules/no-migrated-system-query-owner.test.ts**
- Registration: **apps/web/eslint.config.mjs**
- Wiring tests: **apps/web/scripts/lib/migrated-system-query-owner-wiring.test.ts**

Each diagnostic is attached to the AST node that introduces the supported
mirror. It names the affected resource, ESLint source location, and supported
replacement hook:

| Resource | Retired direct System field | Former setter | Former default | Direct hydration writes | Query replacement |
| --- | --- | --- | --- | --- | --- |
| About SystemInfo | info | setSystemInfo | info: null | draft.system.info = value; deepMerge(draft.system, { info: value }) | useSystemInfo |
| Database statistics | database | setSystemDatabase | database: null | draft.system.database = value; deepMerge(draft.system, { database: value }) | useDatabaseStats |
| Backup list | backups | setSystemBackups | backups: { items: [], loaded: false } | draft.system.backups = value; deepMerge(draft.system, { backups: value }) | useBackups |
| Disk usage | diskUsage | setSystemDiskUsage | diskUsage: null | draft.system.diskUsage = value; deepMerge(draft.system, { diskUsage: value }) | useDiskUsage |

The backup compatibility type **SystemBackupsState** and its named barrel
export are also retired owner forms. Its former value shape was
**{ items: SnapshotInfo[], loaded: boolean }**. Types such as SystemInfo,
DatabaseStats, SnapshotInfo, and DiskUsageResponse remain valid for Query and
API use; the rule does not ban imports or type names.

## Owner syntax and scope

ESLint configuration applies the rule only to these existing owner files:

- **apps/web/lib/state/slices/system/types.ts**
- **apps/web/lib/state/slices/system/system-slice.ts**
- **apps/web/lib/state/slices/system/index.ts**
- **apps/web/lib/state/hydration/hydrator.ts**

The rule recognizes AST structure within those paths, not text occurrences.
It uses the four configured filenames as the outer boundary and the structural
owners and lexical bindings below as the inner boundary. A matching property
name by itself is never sufficient.

| File | Structural owner and supported shapes |
| --- | --- |
| `lib/state/slices/system/types.ts` | Match the exact `SystemSliceState` type alias and inspect only direct members of its direct `system` property type literal. Match the exact `SystemSliceActions` type alias and inspect only its direct members. Reject the four retired data fields and four former setter names there. Reject only the exact `SystemBackupsState` type-alias declaration in this file. Do not follow aliases to reach these nodes. |
| `lib/state/slices/system/system-slice.ts` | Match the exact `defaultSystemState` variable declaration and inspect only direct members of its direct `system` object initializer. Match the exact `createSystemSlice` variable declaration and inspect direct action properties only on its returned object expression; reject the four retired action names even if the declaration has no `set` parameter. For writes, resolve `set` to the first parameter binding of `createSystemSlice` and inspect direct `set` calls in a returned action function's body, including expression-bodied returns and block-bodied calls in ordinary blocks/conditionals. The first `set` argument must be an inline function/arrow recipe. A write must resolve the root identifier to that callback's first parameter binding (named `draft` in production) and pass through the direct static `system` member. Compare lexical bindings, not identifier text. Do not enter nested functions or callbacks; ignore same-named shadow parameters and local unrelated objects. |
| `lib/state/slices/system/index.ts` | Reject only a named `export type` specifier for `SystemBackupsState` from the local types module. Other exports and imports are allowed. |
| `lib/state/hydration/hydrator.ts` | Match the exact `hydrateState` function declaration and its first parameter binding (`draft: Draft<AppState>`). Inspect its body and ordinary nested blocks/conditionals, but do not enter nested functions or callbacks. A direct assignment must target `draft.system.<retired-field>`, with `draft` resolving to that first parameter binding. An inline merge finding requires the callee to resolve to the named `deepMerge` import from `./merge-strategies`, the call to be inside `hydrateState`, argument one to be the exact `draft.system` rooted in the authoritative parameter binding, and argument two to be an object literal with a direct retired-field property. Same-named local `draft`, `system`, or `deepMerge` bindings and nested unrelated calls do not qualify. |

Property names may use identifier syntax (`info`), quoted string syntax
(`"info"`), or a computed static string literal (`["info"]`) at the
positions listed above. These forms are required and tested for properties and
member expressions. Template expressions and computed expressions whose value
is not a literal are unsupported. The rule checks direct members only; it does
not expand object spreads. Comments, ordinary strings, template text, Query
keys, type imports, and unrelated objects do not create findings. A property
with the same name under another root or under `system.storage` is not a direct
System snapshot owner.

The exact accepted owner nodes and binding identities matter. In
`system-slice.ts`, the default must be the direct `system` object inside the
exact `defaultSystemState` initializer; action properties must be direct
members of the object returned by the exact `createSystemSlice` declaration;
and a mutation must call the actual `set` parameter with an inline recipe as
its first argument. The action may return that call directly or invoke it in
its block body. In `hydrator.ts`, writes must use the first parameter binding
of the exact `hydrateState` declaration and remain within its non-nested
function body. The `deepMerge` reference must resolve to the production import.
A nested function declaration, callback, shadowed parameter, local unrelated
object, or same-spelled local helper remains outside the rule even when it
contains a retired key. Structural declarations are checked only in their
owning file, so a `SystemBackupsState` alias in `hydrator.ts` is allowed. The
backup type and barrel export checks use their exact declaration/export nodes,
not matching word sequences elsewhere.

Current jobs, metrics, updates, retention, storage policy, overview, analysis
revision, runs, quarantine, and storage.disk remain valid Zustand state.
UI drafts, mutation feedback, event and job streams, resource-local POST errors,
control-plane no-store process probes, and legitimate Query data imports or API
calls remain permitted. No general ban on fetch, Zustand, Query, types, or
snapshot vocabulary is part of this rule.

## Supported limitations

The rule does not evaluate dynamic computed keys, template expressions,
indirect/local type aliases, object spreads, values passed through helper
functions, renamed semantic owners, or mirrors in files outside the four
guarded paths. It does not follow arbitrary data flow through HydrationState,
generic `deepMerge` values, or `Object.assign` payloads. For example,
`deepMerge(draft.system, state.system)` remains allowed because its payload is
not an inline object literal. These shapes remain outside the enforceable
contract and require design review; they are not grandfathered exemptions.
The rule cannot prove all architectural ownership.

## Registration and verification

The rule is registered as an error only for the four owner paths. The wiring
suite uses ESLint's actual config and API. For each guarded path, it asserts
that `calculateConfigForFile` resolves the rule to error severity, then calls
`lintText` with that exact production `filePath` and a TypeScript fixture that
reproduces that file's supported owner structure. It asserts the emitted
finding's rule ID, error severity, source line, resource name, and replacement
hook. Its per-file positive matrix is:

| Production `filePath` | Required `lintText` positive fixture |
| --- | --- |
| `lib/state/slices/system/types.ts` | Add each retired data property to `SystemSliceState.system`, each former setter member to `SystemSliceActions`, and the exact `SystemBackupsState` alias. |
| `lib/state/slices/system/system-slice.ts` | Add each retired property to `defaultSystemState.system`, each former setter name to the returned object, and a direct write for each retired field through the actual `set` recipe parameter in a block-bodied action. |
| `lib/state/slices/system/index.ts` | Add the exact named type re-export for `SystemBackupsState`. |
| `lib/state/hydration/hydrator.ts` | For each retired field, test both a direct assignment through the actual `hydrateState` draft parameter and a direct property in an inline `deepMerge` payload rooted at that draft's `system`. |

Every positive fixture is linted under the exact owner filename through the
real config, not a path alias or direct rule invocation. Assertions for all
four resolved severities and all four positive paths make removal, disabling,
or omission of any owner path fail the test suite. The current production
owner files are also linted through that config and must produce no finding.
The guard is not configured on the four Query hook paths:
`apps/web/hooks/domains/system/use-system-info.ts`,
`use-database-stats.ts`, `use-backups.ts`, and `use-disk-usage.ts`.

RuleTester and real-config `lintText` fixtures cover each resource's state
field, default, action type/name, slice recipe write, hydration assignment, and
explicit hydration merge, plus the backup alias and export. They cover
identifier, quoted, and computed
static-string keys in supported positions. Negative fixtures cover current
jobs, metrics, updates, retention, storage policy/overview/runs/quarantine,
especially `storage.disk`; legitimate Query types, calls, and hook ownership;
independent control-plane no-store process reads; unrelated domains, roots, and
nested object fields; shadowed `draft`, `system`, `set`, and `deepMerge`
bindings; nested function declarations and callbacks; owner declarations on a
different guarded filename; and snapshot terms in comments, ordinary strings,
and template text. They also cover harmless formatting.
No baseline, rule exemption, broad suppression, or test-only registration is
introduced.

The existing web pre-commit hook runs ESLint only on changed `.ts`, `.tsx`,
`.js`, and `.jsx` paths. It covers edits to guarded TypeScript owner files; it
does not select the `.mjs` rule or `eslint.config.mjs` paths. The existing
`prettier-format` hook does include `.mjs`. The wiring test must run in the
normal web test suite, and the rule/config must run in normal web lint. CI's
`Frontend Tests Passed` gate runs full web lint, typecheck, and tests. The
Python architecture hook and `make lint-architecture` do not run this rule.
No wider pre-commit hook change is needed.

For a red/green check at the production boundary, temporarily add a supported
retired field to the real `defaultSystemState.system` object in
`system-slice.ts`, run targeted ESLint on that file, confirm the
resource-specific diagnostic, restore the exact file contents, and confirm
targeted ESLint passes. This complements the real-config `lintText` matrix for
all four production filenames; standalone RuleTester strings alone are not
sufficient wiring evidence.

The rule traverses only the configured AST owners and adds no whole-tree text
scan. Three pre-registration full web-lint runs measured 55.14, 52.83, and
72.24 seconds (median 55.14 seconds). After registration, run the same three
full web-lint measurements with the same checkout, Python `perf_counter`
runner, and command; report each result and the median alongside the baseline.
Record the lintText/wiring-suite runtime separately if practical. The
post-registration and incremental runtime costs remain unmeasured at this
design checkpoint.

## Related decisions

- [Bounded frontend ESLint architecture guard](../../../decisions/2026-10-08-bounded-frontend-eslint-architecture-guard.md) records the selected tooling boundary and alternatives.
- [Architecture lint budgets and compatibility expiry](../../../decisions/2026-08-01-architecture-lint-budgets.md) continues to define Python architecture rule baselines.
