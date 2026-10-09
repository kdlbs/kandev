---
created: 2026-10-08
status: implemented
requirements:
  - REQ-OFFICE-CONFIG-SYNC-006
system_design:
  - ../../specs/office/system-design/config-sync-surfaces.md
legacy_specs: []
---

# Implementation Plan: Preserve Office configuration drafts during saving

## Overview

Preserve the next intended Office configuration edit when an earlier Save
acknowledgment arrives. One sequential work order adds independently authored
rendered regression coverage, corrects the acknowledgment boundary, and runs
targeted checks after ROOT explicitly releases this reviewed package.

Office owns the config-sync settings surface through
[REQ-OFFICE-CONFIG-SYNC-006](../../specs/office/requirements/config-sync-surfaces.md).
The repair conforms to the existing shared
[AC-UI-SETTINGS-MANUAL-SAVE-001.4](../../specs/ui/requirements/settings-manual-save.md)
and [ADR 0046](../../decisions/0046-settings-route-save-coordinator.md).
Those remain the owners of save and navigation policy. Their requirement is
referenced here rather than duplicated or claimed by Office design frontmatter.

## Confirmed cause and evidence

`handleSave` awaits POST, then unconditionally calls `reset(saved)`. The real
Directory input remains enabled, so a newer form revision can exist at that
point. Reset destroys it and removes the real coordinator's dirty state.

ROOT's accepted disposable proof rendered the real section/hook/store/provider/
coordinator with only external transports mocked. Submitting `submitted`, then
typing `newer-unsaved` while POST was held, reached both soft assertions and
failed on input retention and `hasDirty`. Unchanged-success and rejected-save
controls passed. Receipt: native28b321/session2822/ACTUALcaa312 exit1, original
wrappers2120737/2120759 joined and fresh absent; proof source is protected 0400
under `/tmp/kandev-root-office-save-draft-discovery-20261008/`, SHA256
`adda81315c0acd2761cc0ef9b0a7c7cd07ca456472dcd540d034f51d2d18f389`.
This proof was inspected read-only and must never be replayed, copied, imported,
altered, chmodded, or released by this child.

Observed checkout: `feature/preserve-office-conf-3k5` at
`5aa06bcefe02b6ce7b7c321f7092efe89f0c8992`. The owned hook blob is
`177836c59c246fd85d126757bb5c87269c88832d`, matching the supplied discovery
source. Recheck owned source and reconcile relevant changes if the base advances;
do not replay proof or perform a main-only rebase as validation.

## Scope

### In scope

- Existing hook save acknowledgment and immediately required form-state glue.
- Advance saved baseline on every successful ACK; conditionally adopt canonical
  editable values using equality of current and submitted complete raw forms.
- Preserve newer scalar/provider edits, correct next Save and Reset, normalized
  unchanged success, failure semantics, and shared dirty/navigation observations.
- Owning requirement clarification, bounded design, and this delivery package.

### Out of scope

- Coordinator/navigation policy or engine, backend, schemas, APIs, payload rules,
  layout, copy, Sync Now, Delete, initial load, polling, owner lifetimes, or global
  versioning. Prove causal necessity and checkpoint ROOT before expansion.
- Native agents/tasks/sessions/tabs, model switches, browser/build/full local
  suites, new shared test frameworks, and changes to foreign resources/caches.

## Technical approach

Capture the full raw `OfficeConfigSyncFormState` before POST. Keep payload
construction unchanged. Add a small local acknowledgment operation in
`useOfficeConfigSyncForm` that uses a pure functional update to inspect the
current form: return the response projection only when `formRevision(current)`
equals `formRevision(submitted)`; otherwise retain `current`. Always execute
`setConfig(saved)` outside that operation. Keep existing unconditional `reset`
for unrelated paths and coordinator discard.

| Provider | Identity/transport | ACK behavior | Evidence and fallback |
| --- | --- | --- | --- |
| GitHub | Owner/name; existing POST via `fetchJson` | Full-form conditional canonical adoption | Real rendered scalar/provider cases; missing target retains existing invalid state |
| GitLab | Project path; same transport, GitHub fields omitted | Same full-form rule including cleared fields | Both provider transitions and later provider-specific POST; missing project retains existing invalid state |
| Other | Unsupported by existing tabs/type/backend | No new support | Existing narrowing/validation remains unchanged |

## Desktop and phone assessment

This is pure state/data logic. Both viewports mount the same editable form,
hook, and coordinator. Desktop retains two-column groups and phones the shipped
single-column form. Save/Reset, touch behavior, scroll ownership, safe areas,
native disclosure, focus, and navigation composition are unchanged. The mobile
exemplar is the existing Office provider flow in
`apps/web/e2e/tests/office/mobile-office-config-sync-provider.spec.ts`; the
curated mobile UI language's shared-state/specialized-composition rule applies.
Use the mobile-parity state/data exception: rendered component evidence for the
acknowledgment semantics, no new mobile E2E or browser/build run. No ASCII layout
preview is needed because no rendered composition or controls change.

## Tests

New file:
`apps/web/app/office/workspace/settings/components/office-config-sync-save-drafts.test.tsx`.
Render the real section, hook, StateProvider/createAppStore, SettingsSaveProvider,
TooltipProvider/ToastProvider as required, and actual inputs. Keep APIs and
navigation guard real; mock only external transports such as `fetchJson`.
Observe real `useSettingsSaveCoordinator` values from a minimal test observer.
No internal module mocks, alternate form, synthetic contributor, custom store,
or production setters called from tests.

| Planned behavioral test | Contract/evidence |
| --- | --- |
| `retains a newer directory draft and dirty contributor after an older acknowledgment` | Shared .4; actual input value and `office-config-sync` dirty state both checked after ACK |
| `saves the retained draft on the next explicit Save` | Shared .1/.4; exact first/second POST bodies, no automatic third write, clean only after second ACK |
| `resets newer edits to the acknowledged configuration without posting` | Shared Reset policy; entire acknowledged baseline, not pre-save baseline |
| `preserves a newer edit to each scalar field during saving` | Shared .4 and Office .1; branch, directory, polling interval and switch, provider target fields |
| `retains provider changes during saving and uses the new provider on the next Save` | Office .2 and shared .4; GitHub to GitLab and reverse, other identity cleared/omitted |
| `normalizes an unchanged submitted snapshot and becomes clean` | Shared .4/.3; raw target/branch trims, defaulted branch and root directory, actual contributor and `canLeave` |
| `retains edits reverted to the previous baseline until another Save or Reset` | Shared .4; ACK advances baseline even when pre-ACK dirty state temporarily cleared |
| `does not replace the submitted draft when another field changes before acknowledgment` | Shared .4; full snapshot comparison, no subset check or partial normalization merge |
| `retains newer edits and the previous baseline after a rejected save` | Shared failure policy/.3; actual retry/Reset behavior and failure state |
| `refuses save-and-leave when the draft changes before acknowledgment` | Shared .4/navigation; real coordinator `canLeave=false` and real guard does not proceed |
| `continues save-and-leave after an unchanged normalized save` | Shared .4/navigation positive control; real guard proceeds only after ACK |

The first causal test must fail against unchanged production for the observed
input/dirty loss, before the fix is applied. Keep each deferred POST explicitly
settled and clean up mounted trees, guard state, and any timers. Use causal
request observation and promise settlement; no sleeps, timeout inflation,
race winners, swallowed assertion failures, or weakened dirty checks.

Existing suites provide focused compatibility checks for hook behavior,
component controls, and API payloads. The mocked-controller component suite and
mocked-contributor hook suite cannot replace the new rendered integration.

## E2E tests

Existing Office provider specs cover configured/empty forms, supported providers,
and shared saving on desktop (`office-config-sync-provider.spec.ts`, configured
desktop project) and phone (`mobile-office-config-sync-provider.spec.ts`,
`mobile-chrome`). They do not prove delayed ACK preservation. That regression
is proven through the full rendered component chain at its state boundary.
No Playwright changes or runs are scheduled for this state-only repair.

## Work orders

- [x] [Task 01: Preserve Office save drafts](task-01-preserve-save-drafts.md)

Exactly one work order, sequential, with no dependencies.

## Validation and release boundaries

DESIGN ONLY now: cheap docs checks and read-only inspection. No install, product
tests, ESLint, typecheck, build, or heavy hooks. Leave all package files
unstaged/uncommitted and actually end before ROOT's later reviewed-package
implementation INTERRUPT. No approval/model question and no callback ACK gate.

Later implementation and heavy commands require ROOT's exclusive lease.
Execute checks sequentially and retain each original native start/session/full
terminal and owned PID/PGID/startticks. If dependencies are missing, allow one
`corepack pnpm@9.15.9 install --frozen-lockfile` from `apps` under that lease.
Preserve dependency/cache state; no reinstall when present. Resource, transport,
unknown, out-of-scope, or repeatedly failing checks checkpoint ROOT rather than
being relabeled PASS. Follow the live task plan's publication and merge barriers;
publication authorization does not grant SERIAL MERGE.

## Verification results

Design-only validation on 2026-10-08:

- `python3 scripts/list-docs.py validate`: exit 0; 365 decisions and 1476
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: exit 0; 36 documentation-linter
  tests passed. These are cheap docs checks, not product tests.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specification files
  passed. Catalog discovery includes both owning surfaces documents.
- `git diff --check` and explicit `git diff --no-index --check /dev/null`
  for all three new files: clean. All five docs files remain unstaged/uncommitted.
- Repository `validateCoverage` preflight: actual docs-only diff `exempt`,
  `ok=true`; prospective owned-hook change with this package `covered`,
  `ok=true`, exactly one work order, no errors. Prospective coverage is a
  document-reference check, not implementation or product-test evidence.
- The initial bare `node` preflight could not start because Node was absent
  from the shell PATH. Existing mise Node 24.21.0 was then used directly, with
  no install or environment mutation. An initial wrapper expected `covered`
  for the docs-only diff and exited 1 on its valid `exempt` result; the corrected
  preflight records actual exemption and prospective coverage separately.
- Owned hook blob remains `177836c59c246fd85d126757bb5c87269c88832d`;
  no production/permanent-test file changed.

Implementation validation after ROOT's reviewed-package release:

- The independently authored rendered causal RED failed against unchanged
  production on all three assertions: live Directory was replaced by the older
  submitted value, coordinator `hasDirty` was false, and its real Office
  contributor was clean. Original `red` receipt: exit 1, actual causal evidence.
- Final GREEN: the four work-order files passed, 44 tests total (20 new rendered
  cases and 24 existing compatibility cases). Only external transport is mocked;
  the real component, hook, store/providers, coordinator, guard, and inputs run.
- Affected-file ESLint with zero warnings, direct web typecheck, `i18n:check`,
  and `i18n:ratchet`: each exit 0 in a separate bounded sequential invocation.
- The first full GREEN found two incorrect interval-label selectors; corrected
  to the actual input label. ESLint found test grouping/repeated-string warnings;
  fixed with smaller groups/local constants. No behavioral assertion weakened.
- Dependencies were initially missing; exactly one pinned pnpm 9.15.9 frozen
  install ran from `apps`, exit 0, without lockfile changes. All product checks
  used Node 24.21.0, a 4 GiB Node heap, GNU 180-second timeout/10-second kill
  grace; Vitest used one worker. No timeout or resource failure occurred.
- Original native handles, complete terminal output, and owned process
  identities are retained in the durable task plan and
  `/tmp/kandev-child89-save-drafts-20261008/`. Each check was joined with fresh
  owned-group absence before the next heavy check. Protected discovery proof
  remained untouched. No browser, build, or broad local suite ran.

Task 01 is implemented. Final documentation and normal active commit hooks are
publication checks; hosted checks/review and ROOT's separate SERIAL MERGE grant
remain delivery gates in the durable task plan.

## Documentation impact

The shared requirement already defines the intended save/Reset/navigation
behavior. Office's surface requirement links it; the new Office design defines
the local implementation boundary. Existing shared rollout and action-redesign
plans remain completed historical packages, not reopened deliveries. Public
Office config-sync docs, README, and screenshot catalog were assessed: no
configuration keys, API, actions, layout, labels, or screenshots change. This
package records a conformance repair; no public-document edit is required.

## Risks

- Comparing the callback's closed-over form with itself would always accept the
  ACK; compare current queued state in the form owner.
- Skipping baseline advancement would corrupt later Reset and dirty comparison.
- Comparing normalized payloads or only a field would lose raw/provider edits.
- Suppressing normalization for unchanged snapshots would leave successful
  submissions falsely dirty. The real coordinator test must observe this.
- Cross-workspace/load/poller concurrency is not proved by these tests and is
  outside this repair; evidence requiring expansion must be checkpointed.
