---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-PROFILE-SAVE-CATALOGUE-001
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
legacy_specs: []
---

# Implementation Plan: Concrete profile save catalogue preservation

## Overview

Publish an accepted concrete profile save against current browser inventory so
it cannot restore a deleted sibling or overwrite unrelated current state.
One sequential work order covers the active editor, independent rendered
regression, scoped checks and documentation results. ROOT reviewed the design checkpoint and explicitly released implementation
in the same primary session on 2026-10-10.

## Scope

In scope: the accepted publication branch of `useProfileSave` and its concrete
`ProfileEditor` caller, current owner/target absence, current unrelated nested
membership/metadata and options, actual choice consumers, and compatibility
with existing editor revision/draft rules.

Excluded: delete/duplicate/creation/dynamic writers, global handlers and
availability/eligibility policy, backend/API/persistence/tombstones, new server
ordering claims, copy, markup, navigation and mobile geometry. No delegation,
new sessions, model switch, protected ROOT source access or foreign resource
changes. Other delivered catalogue plans retain their statuses and scope.

## Technical approach

The independent source inspection at HEAD
`ac25d0c22e8e7db8c57cddd5098ba6946dd7d4af` confirms
`components/settings/agent-profile-page.tsx::ProfileEditor` uses
`agent-profile-page-state.ts::useProfileSave`. The accepted PATCH branch maps
captured `settingsAgents`; option reconciliation then rebuilds a deleted sibling
from that stale nested list. ROOT reports native75344/join d7edb6 exit 1 with
one causal failure and two controls passing. That is a supplied receipt, not
this task's independent RED.

Use the mounted store API inside the existing save hook after response acceptance.
Update only a present target under a present current owner, and merge only the
target projection into current options with the existing newest-option helper.
Retain setters and metadata/version semantics. Remove only save's stale
inventory/callback inputs; the caller still supplies them to delete. No new
hook is required; any necessary new hook belongs in `hooks/domains/settings/`.
Keep the existing target revision guard, PATCH adapter/normalizer, baseline,
draft, conflict, permission, enabled omission, Cursor MCP, force and errors.

Agents owns the requirement/design pair. Platform's
[editor reconciliation](../../specs/platform/system-design/agent-settings-parity.md#editor-reconciliation)
and Agents' [permission integrity](../../specs/agents/requirements/permission-control-integrity.md)
are dependencies, not rewritten contracts. No new ADR or public documentation
is needed for this local conformance fix; existing user workflows remain valid.

## Tests

Author `apps/web/components/settings/agent-profile-save-catalogue.test.tsx`
independently after release. Test names and mappings:

| Test | Criteria | Evidence |
| --- | --- | --- |
| `keeps a live-deleted sibling unavailable after save acknowledgement` | .1, .5 | Real editor/save/API/store/WS/picker, nested absence and actual option absence before/after ACK; causal RED first |
| `publishes an ordinary accepted save` | .3 | Returned normalized name/revision and real choice label, clean baseline; pre-fix control PASS |
| `retains live deletion when PATCH is rejected` | .4 | No publication, real unavailable choice, coordinator failure/draft retained; pre-fix control PASS |
| `preserves current sibling and independent inventory` | .2, .5 | Updated sibling, added same-owner member, other agent metadata, independently held newer/disabled options and task/subtask labels |
| `does not publish a removed target or owner` | .2 | Separate current target/owner absence cases, no insertion or unrelated option rebuild |
| `retains newer revision and edits made during save` | .3 | Existing acceptance/draft behavior through real editor, late response rejected and in-flight edit retained |

Adapt the two existing direct save-hook payload tests to the store dependency,
preserving Cursor MCP request assertions. Run the changed suite plus nearby
page, reconciliation and picker compatibility suites exactly as the work order
lists. No backend, whole-app build or broad passing suite replay.

## Rendered flow and mobile parity

Real component integration is the end-to-end evidence for this state/data path:
the concrete editor and shared coordinator issue the real PATCH adapter, live
handler removes a sibling, and actual selectors retain Unavailable after ACK.
Actual task/subtask derivation and controls prove retained eligible labels and
disabled eligibility. External transport and documented noncausal capabilities
may be isolated; causal owners must be real.

The mobile-parity narrow state/data exception applies. Shared publication changes
no rendered composition, touch, scroll, copy or navigation. No ASCII layout
redesign, untouched mobile Playwright replay or production rebuild is required.
If implementation expands that scope, reassess the package first.

## Work orders

- [x] [Task 01: Publish concrete saves against current inventory](task-01-current-save-publication.md)

## Changed-path coverage

The four design artifacts are the existing Agents
`requirements/creation-catalogue.md` and `system-design/creation-catalogue.md`,
this manifest and its sole sibling order. At implementation, the order owns
`agent-profile-page-state.ts`, `agent-profile-page.tsx`, the new rendered suite
and the adapted `agent-profile-page-state.test.ts`. All paths are explicit in
the order. Inventory tracked and untracked actual paths for the real repository
coverage evaluator; also validate the prospective production trigger at design.
Never present prospective paths as an actual production diff.

## Verification results

Design checkpoint checks passed on 2026-10-10 under ROOT's limited lease:

- `python3 scripts/list-docs.py validate`: 368 decisions and 1498 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
  Native session 57323 joined with exit 0, terminal chunk `81464b`.
- Actual tracked-plus-untracked coverage: exactly the four design artifacts,
  `exempt`, no errors. Prospective production-trigger reference coverage:
  `covered`, no errors, exactly the one work order declared above.
- `git diff --check`: passed. Requirement 10,622 bytes; design 22,935 bytes,
  below their limits. No size condensation or exceptions required.

All owned commands joined. No scratch files, production/permanent tests,
dependency installation, frontend statics, commits or publication. The limited documentation lease was returned at that handoff. ROOT later
released implementation and a separate local execution lease.

## Risks

- A hook-only test with mocked response acceptance misses the active editor
  reconciliation and actual selectable-option resurrection.
- Flattening all current nested rows into options can still replace unrelated
  independently received metadata; reconcile only the accepted target.
- Target/owner absence must skip publication without introducing another
  baseline, conflict or server ordering policy.
- Test capability initialization must settle real prerequisites and clean up
  deferred requests/subscriptions; setup errors are not causal RED evidence.


### Implementation verification (2026-10-10)

The sole work order is done. Production scope is limited to current target
publication in the existing save hook and two removed caller inputs; delete,
revision reconciliation and all other writers are unchanged.

- Independent RED: native60600 joined exit 1/chunk `a1b433`, exactly one
  post-ACK unavailable-label failure and two controls passing. Actual concrete
  page, coordinator, API adapter/normalizer, live deletion/store/picker are real;
  only external HTTP is isolated. No protected ROOT source was accessed.
- Initial GREEN: native37120 joined exit 0/`a6d871`, 3/3 tests passed.
- Final affected command in the work order: native78946 joined exit 0/`243800`,
  five suites/43 tests passed, including eight new integration cases. Real
  task/subtask selections, disabled/Office exclusions, current owner/target
  absence, latest sibling/options, newer revision and in-flight draft pass.
- Scoped ESLint passed with zero warnings (native52761 exit 0/`9ab11e`;
  final combined native24041 also completed ESLint before typecheck).
- Four-file Prettier check passed, exit 0/`8dbfa6`.
- i18n ratchet passed, native92510 joined exit 0/`9ee478`.
- Normal web `pnpm run typecheck` passed, native69862 joined exit 0/`723f04`.
  Earlier typechecks identified only incomplete test wire-fixture annotations;
  the fixture was corrected without changing production contracts.
- Documentation commands passed, native57608 joined exit 0/`b0f2bd`: catalogue
  368 decisions/1498 specs, 36 spec-validator tests, full lint and whitespace.
- Actual tracked-plus-untracked coverage passed as `covered`, no errors, all
  eight changed paths, exactly one work order (`467c51`).

Initial fixture cleanup, selector property/name assertions and wire annotation
failures were corrected before final checks; they are not causal RED claims.
The independent RED import emitted a KaTeX quirks warning and React DevTools
notice; no setup/unhandled errors. Final affected tests were error-free.
No layout/touch/navigation change: the reviewed mobile state/data exception
uses rendered component flows; no untouched mobile E2E or full build replay.
No public docs update is needed because user workflows and terminology remain
valid. Install/native52060 joined exit 0, using the frozen lockfile once.

All local check handles have joined. Normal commit hooks and hosted validation
are delivery steps after this completed implementation record.
