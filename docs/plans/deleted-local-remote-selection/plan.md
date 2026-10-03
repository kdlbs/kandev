---
created: 2026-10-02
status: implemented
requirements:
  - REQ-WORKSPACES-REMOTE-RESOLUTION-001
system_design:
  - ../../specs/workspaces/system-design/remote-repository-resolution.md
legacy_specs: []
---

# Implementation plan: Remote selection after local checkout deletion

## Overview

Correct remote repository selection so a deleted local checkout does not strand
new tasks. One sequential work order implements the selector and proves the
service-to-preparation flow. Production changes and regression coverage are complete.

CI follow-through also updates desktop and mobile GitHub URL, file-link, and
pasted-URL subtask fixtures to carry matching origins. Offline remotes remain
available through checkout-local Git URL rewrites, restored after shared
checkout use. All 13 reported failures pass locally with retries disabled;
delivery continues through exact-head CI and review.

The workspace system owns the selection contract because it owns repository
registrations. Existing task-worktree recovery contracts remain separate.

## Confirmed root cause and reproduction

`FindOrCreateRepository` selects the earliest provider-identity row without
checking its local availability. After a local checkout is manually deleted,
`ResolveRepositoryRef` still returns that local row. The executor intentionally
skips cloning `source_type=local`; preparation therefore receives its stale path.

On 2026-10-02, temporary
`TestRepro_RemoteSelectionAdoptsDeletedLocalCheckout` created a real Git
checkout and registered it through `CreateRepository`, removed the directory,
then resolved `https://github.com/acme/widgets.git`. It failed the expectation
that remote selection supplies a cloneable source: the same ID returned with
`created=false` and `source=local`. The throwaway file was removed after diagnosis.

Both existing executor controls passed:
`TestResolveTaskRepoInfo_ReClonesWhenLocalPathIsNotAGitRepo` and
`TestResolveTaskRepoInfo_DoesNotReCloneLocalSourceTypeRepo`. This locates the
correction in selection, without weakening local ownership in the executor.

## Scope

### In scope

- Remote locator resolution for a saved checkout directory that is absent.
- Eligible matching candidate selection and managed registration fallback.
- Preservation of skipped local registrations and explicit local behavior.
- Identity isolation, repeat/concurrent selection, and launch preparation proof.

### Out of scope

- Existing materialized worktree recovery and missing local Git objects.
- Recreating a user's local path or changing its registration/source type.
- New UI, APIs, clone protocols, credentials, flags, or database migrations.
- Repairing invalid checkouts and inaccessible mounts.

## Technical approach

Extract remote-candidate eligibility and fallback lookup into a small private
helper in `internal/task/service/remote_repository_resolution.go`. Integrate it
into `FindOrCreateRepository` in `service_resources.go` under `repoResolveMu`.
Only requests with a remote URL and no explicit local path use this policy.
Keep existing field backfill, creation, and rollback ownership semantics.

Validate provider scope on the first candidate and alternatives. Unscoped
remote selection must exclude scoped rows returned by the legacy SQL lookup.
Validate each existing local checkout's current origin against the requested
remote identity, accepting equivalent HTTPS and SSH transports.

After the earliest local candidate is proven absent or the first lookup returns
a foreign scope, enumerate raw workspace rows, deterministically select an
eligible row, or use the current provider-row creation path. Repeated calls
must find a previously created managed row behind the stale local candidate.

| Input/provider shape | Intended behavior | Evidence |
| --- | --- | --- |
| GitHub HTTPS or SSH, including `github_url` compatibility | Skip deleted local match; reuse/create managed row | Service table tests and backend integration |
| Built-in GitLab, including trusted self-managed host | Same eligibility with normalized host isolation | Service table tests |
| Azure DevOps locator | Same selection; retain existing clone-auth path | Service resolution test; no new PAT transport behavior |
| Trusted plugin descriptor | Preserve scope/immutable ID and exact clone URL | Scoped candidate tests |
| Explicit local path or repository ID | Existing ownership behavior | Existing controls and service negative cases |
| Permission, invalid Git, or changed canonical identity | Return inspection failure without modifying local registration | Focused filesystem cases |
| Unsupported or untrusted provider locator | Existing resolver rejection | Existing remote resolution tests |

## Tests

In `remote_repository_resolution_test.go`, add
`TestResolveRepositoryRef_RemoteSelectionSkipsDeletedLocalCheckout` as the
permanent version of the failing reproduction (AC-001.1, .3).
Use a real checkout and deletion, not a fabricated missing path alone.

Add cases for live local reuse, deleted local plus live local, deleted local
plus managed provider, only deleted local matches, repeat/concurrent fallback,
explicit local/ID preservation, canonical/permission validation, and host/scope/
workspace isolation (AC-001.2-.5). Assert sentinels and original dependent rows
so identical data cannot mask selection of the wrong source.

## End-to-end evidence

Add `TestRemoteSelection_DeletedLocalCheckoutPreparesManagedWorkspace` in
`internal/orchestrator/executor/executor_remote_selection_integration_test.go`
(AC-001.1, .3, .6). Construct a real task service and SQLite repository, resolve
the remote locator, persist its task attachment, and pass it through executor
repository preparation and real worktree creation. A controlled clone adapter
uses a disposable local bare origin; the final agent boundary uses an existing
fake. Assert clone/branch/checkout success and original-directory absence.
Add a clone-failure case that proves no agent startup and no local-path mutation.
This is backend end-to-end evidence; no rendered controls change, so a browser
test would add an unrelated boundary.

## Work orders

- [x] [Task 01: Select a cloneable source after local deletion](task-01-select-available-remote-source.md)

## Verification results

- Temporary service reproduction: failed as expected, exposing deleted-local adoption.
- Two targeted executor controls: passed.
- Permanent deletion regression: failed before the correction and passed afterward.
- Final race checks: both `internal/task/service` and `internal/orchestrator/executor` passed.
- Scoped fallback race regression: passed after adding explicit same-host,
  wrong-scope, unscoped-legacy, and wrong-immutable-ID candidates.
- Backend integration: managed cloning, real worktree creation, requested base
  commit, and agent startup passed; authentication failure and cancellation
  prevented agent startup and preserved the original local registration.
- Changed-code lint: zero issues for both affected packages.
- Specification catalog validation, 36 specification-linter tests, full spec lint,
  62 public-doc validator tests, validation of 47 public docs pages, PR-documentation
  coverage (`covered`), and whitespace checks passed.

Public recovery guidance was added to `docs/public/tasks-and-workflows.md`.
The paired requirement and design are active/current.

## Review remediation

Addressed checkout-origin validation and both directions of provider-scope
isolation. The new unscoped-admission and changed-origin regressions failed
before remediation and passed afterward. Scope checks cover initial lookup,
fallback, reuse, creation, and repeated convergence. Origin checks cover
retargeted/replaced checkouts, missing or invalid origins, preserved rows,
explicit ID selection, and equivalent HTTPS/SSH transports across providers.

Post-remediation race checks passed for both affected packages (service:
65.532s; executor: 11.317s); changed-code lint reported zero issues. The spec
catalog, full spec lint, public-doc validation, and whitespace checks passed.
Updated the requirement, design, and public recovery guidance together.
Remote CI and subsequent review remain pending until the updated head is pushed.

## Risks

- Initial lookup and raw workspace enumeration must preserve scoped and host identity rules.
- Missing paths can represent unmounted storage; preserve the original local row.
- Filesystem disappearance after selection can still fail ordinary preparation.
- Fallback must not copy local secrets or machine-specific scripts.
- Rollback must only delete registrations actually created by that request.

## Additional CI remediation

After the origin fixtures were corrected, CI exposed concurrent navigation
fixture checkout preparation and premature history geometry sampling. The
helper now settles each shared-checkout turn before starting another task,
and the spacing test polls its existing measured-layout expectations. The
helper regression passed after failing before the correction; all 17 affected
desktop/mobile browser tests passed with retries disabled under CI resource
limits. Observer-delivery diagnostics reproduced and explained the spacing
failure, then were removed. No production or durable behavior changes were
needed. See the work order for verification evidence.

Full CI catalog validation also required placing the fixture unit regression
in `e2e/helpers/`, outside Playwright's browser-test root. The moved unit test,
all-project discovery, and duration-aware manifest generation passed.

The following CI run exposed the same premature geometry sample in the phone
history test. Holding observer delivery reproduced its exact 20px sibling
gaps; measured geometry returned to 2px/10px/-4px. The phone assertion now
polls the existing expectations at both widths. Ten resource-bounded repeats
and the 16-test desktop/mobile history and preceding-mobile sequence passed
with retries disabled. Production behavior and layout remain unchanged.


The next E2E run passed all shards but exposed three fixtures that needed a
retry. Controlled reproductions confirmed each setup assumption: a navigation
regression inherited an unrelated dirty tracked file, Quick Chat selected a
different registered repository by position, and the default-layout assertion
inherited an enabled Todos preference. The navigation regression now uses an
owned clone and verifies that the shared checkout is preserved. Quick Chat
selects its seeded repository by ID, and the default-layout test explicitly
establishes the default Todos preferences. Production behavior and existing
assertions remain unchanged. See the work order for verification evidence.
