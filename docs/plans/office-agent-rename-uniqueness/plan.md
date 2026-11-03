---
created: 2026-10-26
status: implemented
requirements:
  - REQ-OFFICE-AGENT-NAMES-001
system_design:
  - ../../specs/office/system-design/agents-01.md
legacy_specs: []
---

# Implementation Plan: Office agent rename uniqueness

## Overview

Reject an Office name already occupied by another identity in the same
workspace before saving the update. Deliver this as one sequential service,
HTTP, and persistence/export regression slice. The Office
[agent requirement](../../specs/office/requirements/agents.md#req-office-agent-names-001-workspace-agent-name-admission)
and [identity design](../../specs/office/system-design/agents-01.md#workspace-agent-name-admission)
own the behavior.

The reviewed slice and the accepted CEO name-error precedence remediation are
implemented and locally verified. Required-CI browser fixture corrections are
included in the same work order; their limited validation is recorded below.

## Scope

### In scope

- Reuse existing exact workspace name admission for ordinary updates, excluding
  the edited agent's ID.
- Classify a duplicate name with a service sentinel and the existing HTTP
  validation response machinery. The shared create helper gains the same
  client-error classification for duplicate names it already refuses.
- Prove refusal, persisted-row preservation, and continued configuration export
  through independent tests using the real service and repository.
- Preserve successful unchanged/omitted-name saves, unused names, reuse in
  another workspace, exact string semantics, and the global profile namespace.
- Correct two existing workflowsync test fixtures exposed during required CI:
  wait for automatic sync completion before changing their next-phase inputs.
- Correct the three repeated positional profile assumptions in the existing
  mobile profile-deletion fixture using its exact seeded profile identity.
- Reset the shared review-toggling fixture's seeded checkout and assert its
  exact two-file slate before exercising the existing scroll assertions.

### Out of scope

Schema/index/migration changes, concurrency admission or locking changes,
existing-data repair, exporter workarounds, broad rename refactoring, the
legacy `office/service` facade, production config import/sync reconciliation, execution
profile naming, frontend product changes, browser changes outside the named
test fixtures, dependency changes, and runtime feature flags.

## Confirmed cause and reproduction

The inspected checkout and remote `main` both resolved to
`baa92e5dcf53eca4dac223b0deba5e26c1ee08fb` during design. The source trace is:

1. `backendapp.mountOfficeRoutes` mounts `/api/v1/office` and calls
   `office.RegisterAllRoutes`; `agents.RegisterRoutes` registers
   `PATCH /agents/:id` against `Services.Agents`.
2. `Handler.updateAgent` loads the target and calls
   `AgentService.UpdateAgentInstance` after applying its request fields.
3. `validateAgentCreate` checks name uniqueness, but `validateAgentUpdate`
   omits that check. `Repository.UpdateAgentInstance` then writes the duplicate
   name directly.
4. `ExportManifest` produces agent paths from names; its duplicate-path guard
   rejects the second `.kandev/agents/Alpha.yml`.

Create Workers Alpha and Beta in workspace A through the real AgentService;
export successfully; PATCH Beta's name to Alpha through the registrar. The
current code path admits the rename, permits two Alpha rows, and makes the
next manifest fail. The independently authored permanent regression reproduced
the accepted rename, changed storage, and duplicate-path export failure before
the fix; it passes after validation is restored.

Simply invoking the name helper is insufficient: its current untyped conflict
would use the handler's `500` branch. The sentinel and handler mapping are
part of the same fix.

## Technical approach

In `apps/backend/internal/office/agents/service.go`, add
`ErrAgentNameAlreadyExists`, wrap it when `validateAgentNameUnique` detects a
collision, and call that helper in `validateAgentUpdate` with `agent.ID` as
`excludeID` before the reporting-parent early return. Preserve existing helper
behavior on repository errors.

In `apps/backend/internal/office/agents/handler.go`, map the sentinel with
`errors.Is` to `agent_name_already_exists`. Reuse the create/update handlers'
existing `400` response branches. Do not classify arbitrary service errors as
client validation.

The repository query, storage update, route registration, DTOs, and exporter
are read-only integration dependencies. Add focused tests in new files within
the `agents` package; the HTTP/export test may import `office/config` and
construct a ConfigService over the same real repository. Export does not need
a running agent, filesystem sync, browser, or configuration loader.

## Tests

All listed tests are implemented and included in the passing targeted and
affected-package runs below.

| Evidence | Test and path | Acceptance |
| --- | --- | --- |
| Direct service rejection, unchanged persisted rows, duplicate create classification | `TestAgentNameUniqueness_Service` in `apps/backend/internal/office/agents/service_name_uniqueness_test.go` | `AC-OFFICE-AGENT-NAMES-001.1`, `.2` |
| Accepted unchanged, omitted, unused and cross-workspace names; exact names, deleted rows, global profiles | `TestAgentNameUniqueness_Service` subtests in the same file | `AC-OFFICE-AGENT-NAMES-001.3`, `.4`, `.5` |
| Registered PATCH returns 400; both rows/timestamps and co-submitted fields survive unchanged; manifest retains both configs and revision | `TestAgentNameUniqueness_HTTPExport/reject_occupied_name` in `apps/backend/internal/office/agents/handler_name_uniqueness_test.go` | `AC-OFFICE-AGENT-NAMES-001.1`, `.2`, `.6` |
| Accepted self-name, omitted-name, unused-name and cross-workspace PATCHes persist and export | `TestAgentNameUniqueness_HTTPExport` control subtests in the same file | `AC-OFFICE-AGENT-NAMES-001.3`, `.4`, `.5` |
| Duplicate POST returns 400 with the same stable code and no insertion | `TestAgentNameUniqueness_HTTPCreate` in the same file | `AC-OFFICE-AGENT-NAMES-001.1`, `.2` |
| Duplicate-CEO POST and colliding-name promotion PATCH report name conflicts; distinct-name controls retain the CEO guard; all preserve storage/export | `TestAgentNameUniqueness_HTTPCEOPrecedence` in the same file | `AC-OFFICE-AGENT-NAMES-001.1`, `.2`, `.6` |

The primary collision fixture starts from valid exportable state. Assert the
conflict independently of export, then reread both rows and assert the
manifest; a failed status assertion must not hide the pre-fix storage/export
consequences. Controls use independent fixtures to avoid relying on a refused
request having behaved correctly. A collision test also sets a valid
`reports_to`, exercising the update validator's current early-return branch.

## End-to-end API evidence and mobile boundary

`TestAgentNameUniqueness_HTTPExport` mounts the real registrar under
`/api/v1/office`, submits HTTP through Gin, persists through SQLite, and calls
the real manifest exporter. It supplies the end-to-end evidence for the changed
backend action. It does not claim full backend authentication assembly coverage;
those route guards do not change.

Desktop and phone save actions share this API refusal and saved state. No
rendered, touch, scrolling, navigation, or frontend-state behavior changes, so
the backend correction needs no mobile layout or UI preview. The later required-CI
browser fixture corrections retain the existing desktop and mobile assertions.

## Public documentation impact

No public docs change is needed. `docs/public/office-agents.md` already requires
unique agent names; `docs/public/office-config-sync.md` already documents the
server-owned export manifest. This repair restores those outcomes. Root README
and screenshot catalog describe Office without naming different rename rules;
neither needs an update.

## Work orders

- [x] [Task 01: Enforce Office agent name admission](task-01-enforce-agent-name-admission.md)

The single work order was executed sequentially. No prerequisite work order or
parallel work was required.

## Verification results

Design checks completed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local link/frontmatter inspection: all 24 Markdown links and all referenced
  file paths resolve across the four artifacts.
- The work order's `validateCoverage` preflight, executed with an existing Node
  binary: `ok: true`, `status: covered`, no errors. Runtime paths are prospective
  inputs for this design check; production code has not changed.
- `git diff --check -- docs/specs docs/plans/office-agent-rename-uniqueness`:
  passed. All four artifacts remain unstaged and uncommitted.

Implementation verification on 2026-10-26:

- The work order's exact targeted regression command failed before production
  edits with exit 1: PATCH returned 200, persisted the duplicate name and other
  fields, and blocked manifest export; duplicate POST returned 500.
- The same command passed after the fix with exit 0 and race detection
  (`go test` reported 1.338s).
- The work order's affected `./internal/office/agents` package command passed
  with exit 0 and race detection (`go test` reported 2.084s).
- The exact changed-Go lint command passed with exit 0 and `0 issues`, using
  the canonical 3GiB default.
- One `pnpm install --frozen-lockfile` from `apps` passed with exit 0 to supply
  the active publication hooks; no manifest or lockfile changed.
- Final catalog/specification checks: passed. All 24 local links and all
  frontmatter file references resolve; delivery coverage reports `covered`
  without errors; final `git diff --check` passes.

The requirement and design files retain their broader draft lifecycle status;
this completed delivery covers only `REQ-OFFICE-AGENT-NAMES-001`.

### CEO name-precedence remediation

The existing name-conflict acceptance criterion also applies to a requested CEO
role. The initial validators check CEO capacity before name admission, so
duplicate-CEO POST and Worker-to-CEO rename PATCH requests report the CEO-count
error as HTTP 500. This accepted correction stays within the existing contract.

Independent `TestAgentNameUniqueness_HTTPCEOPrecedence` cases cover both
registered requests and distinct-name CEO controls, including row and export
preservation. The targeted command in the work order failed on the two intended
HTTP 500/name-code assertions before production changes; distinct-name controls
passed. Only then were the existing name checks moved before CEO validation in
both validators, retaining all existing guards and the exact predicate.

Post-correction verification on 2026-10-26:

- Full name-regression selection: exit 0, race enabled, Go result 1.451s.
- Affected agents package: exit 0, race enabled, Go result 2.168s.
- Changed-Go lint: exit 0, `0 issues`, canonical 3GiB default.
- Catalog/specification checks passed; all 24 local links and frontmatter file
  references resolve; delivery coverage is `covered` without errors; diff check
  passed.

No dependency reinstall or broader suite is part of this remediation. Earlier
verification results remain the historical initial-slice evidence.

## Risks

- The shared helper changes duplicate-create HTTP classification from an
  internal error to `400`; pin this intended effect with the POST regression.
- Place the name check before optional reporting validation returns and pass
  the target ID; otherwise parented agents or ordinary self-name saves can
  escape or fail admission.
- The check preserves the existing read-then-write concurrency model. Existing
  duplicates and simultaneous competing writes are not repaired by this work.
- A refactor to compare names globally, normalize strings, or include deleted
  rows would change existing namespace semantics; retain the repository helper
  and its controls.

### Required CI fixture completion correction

Required CI exposed a synchronization gap in the existing workflowsync tests.
`PollSuspended` is persisted before the automatic worker releases its workspace
lock and removes its in-flight entry. Observing the saved flag does not prove
that the first worker has finished; a following rearm tick can lose its
`TryLock`, and metric or clock assertions can race the prior worker.

The delivery scope includes only these additional test paths:

- `apps/backend/internal/workflowsync/circuit_test.go`:
  `TestSyncDueConfigs_FirstKnownFingerprintRearmsSuspendedConfig`.
- `apps/backend/internal/workflowsync/service_test.go`:
  `TestPermanentFailureSuspendsPollingUntilManualRecovery`.

Each fixture now waits, under `automaticMu`, for
`automaticInFlight["ws-1"] == nil` after observing persisted suspension and
before changing provider, metric, or clock inputs. Existing assertions and
one-second eventual-check bounds remain intact. Production workflowsync code,
Office behavior, public documentation, and the product contract do not change.

The hosted failure supplies the original failure evidence. The unchanged local
selection passed both cases at count 20, so the CI failure was not reproduced
locally; it was not rerun to obtain a failure. After the waits, the same
selection passed at count 20 with race detection (Go result 1.248s), and the
affected workflowsync package passed once (Go result 1.322s). Changed-Go lint
passed with zero issues at the canonical 3GiB limit. Catalog/specification
checks, all 24 local links, delivery coverage, gofmt, and diff checks passed.
Hosted CI and review remain external delivery gates.

Three exact failing browser cases also passed once locally with retries
disabled: mobile Stats, mobile review toggling, and desktop review toggling.
Those passes do not clear their required CI failures or establish their causes.

### Required CI mobile profile fixture correction

The mobile profile-deletion case failed repeatedly before its first UI action:
`agents[0].profiles[0]` was undefined. The API may return an empty CLI family
before the family that owns the seeded profile. Per-test setup guarantees and
preserves `seedData.agentProfileId`, not a profile on every returned family.

The prepared change is limited to
`apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts`. A local helper
finds the exact seeded profile across returned agents and asserts one owning
identity. The cancel case uses that profile; the two disposable-profile cases
use its owner and model. Existing UI actions, assertions, timeouts, and API
ordering remain intact. Public documentation and the product contract do not
change.

The hosted failures provide the original fixture failure evidence. All three
mobile cases passed once on 2026-10-27 (11.0s), headless with one worker and
retries disabled, through the guarded managed runner using existing assets.
The selection listing confirmed exactly three cases. No dependency install or
build was repeated. This fixture correction changes no rendered mobile behavior.

A separate existing pseudo-office locale setup case has a confirmed flaky
verdict but an unknown cause. Its one-case initial-trace diagnostic is recorded
separately and does not establish a locale, translation, or product correction.
The unchanged desktop case passed once (5.6s), headless with one worker and
retries disabled. Its initial trace records no locale cookie and English shell
and boot locale on the first navigation; the reload sends
`kandev_locale=pseudo` and receives pseudo shell and boot locale. This passing
trace does not reveal the hosted initial failure's missing request evidence.

### Required CI review fixture correction

The shared review-toggling fixture now resets its seeded checkout with the
existing helper and requires exactly its target and trailing file headers
before the existing scroll sequence. The change is limited to
`apps/web/e2e/tests/review/review-viewed-toggling-flow.ts`, carried byte-for-byte
from reviewed commit `6b84f031a530bdcbdf78da42b97203df6fbd5afe`. No production
scroll utility or shared helper implementation is included.

The reviewed producer's ordered three desktop cases and one mobile case passed;
that limited evidence is reused without replaying the cases here. Earlier
hosted flaky verdicts remain recorded. The separate observation about marking
multiple files remains unresolved and is not claimed fixed by this fixture
change. No public documentation or product contract changes are needed.

The final fixture delivery checks passed: documentation catalog and specification
lint, all 24 local links, delivery coverage with no errors, exact carried-file
identity, and the scoped diff check. Hosted CI remains a separate delivery gate;
local fixture passes do not clear earlier hosted flaky verdicts.
