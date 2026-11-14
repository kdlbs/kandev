---
id: "02-runtime-footprint-projection"
title: "Read-only per-session runtime footprint projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RUNTIME-RECLAIM-003
acceptance_criteria:
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.1
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.2
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.3
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.4
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.5
  - AC-EXECUTORS-RUNTIME-RECLAIM-003.6
system_design:
  - ../../specs/executors/system-design/agent-runtime-reclamation.md
---

# Read-only per-session runtime footprint projection

## Scope and acceptance

Connect the owned-process measurement, existing authenticated control transport,
current lifecycle attribution, workspace read authorization and filtering to the
responsive settings card. Preserve unknown/failed readings and unreadable lower
bounds. Carry counts, bytes, state and existing owner identity only.

This is the observation-only extraction from the original mixed implementation.
It follows the final request-driven behavior: no maintenance tick, metric
registry, inference inventory, reclaim policy, automatic stop, DB write,
operating configuration or new timer is imported. Terminal-reclaim task-01 is
separate and is not a prerequisite for reading memory.

## Affected UI

```text
Workspace settings -> Agent memory in use
  desktop: four totals, wrapping session rows
  phone:   two-column totals, same session rows and measurement states
```

The card reads once per mounted workspace and does not participate in form save.
Seven real locale catalogs and generated pseudo provide the settings copy.

## Verification

Focused Windows checks used temporary explicit overlays outside normal collection:

- Process accounting/ancestry, real hidden-child RSS and busy-owner unknown marker:
  PASS with race detection (1.366s). The actual owned subtree returned two
  processes, 84,516,864 resident bytes and zero unreadable processes.
- Instance attribution and unlocked enumeration: PASS (1.079s). Current exact
  lifecycle binding, stale-instance exclusion and failed/partial/no-reader
  controls: PASS (1.188s).
- Real repository workspace filtering, authorization before measurement and
  incomplete observations: PASS (1.979s).
- Authenticated agentctl control GET plus real ControlClient read/refusal: PASS
  (1.142s); the existing control-token case passed (1.090s).
- Actual workspace GET route with foreign 404/no observation, owned partial row
  filtering and incomplete-row suppression: PASS (1.111s).

The selected Windows packages passed 30 top-level cases in total. The existing
remote-Docker integration case was skipped because its external fixture was not
configured; this is not remote-runtime coverage.

Frontend DTO/card/hook controls passed 16 cases in three explicit temporary
files (5.44s): loading, failure, unknown, known empty, lower bounds, unsupported
commit charge, stale workspace response rejection and real English/Korean copy.
ESLint passed for the five changed TypeScript/TSX files with zero warnings.
These checks were reused rather than repeated during backend validation.

Linux amd64 and Darwin amd64 CGO-disabled compilation passed for the five
observation packages: process, instance, control API, control client and DTO.
No physical Linux/Darwin memory measurement was performed. A broader Linux
task/backendapp CGO-disabled attempt reached existing SQLite references to
sqlite3.Error, ErrConstraintUnique and SQLiteTimestampFormats, which are not
available in that build mode. Those source files are unchanged from the selected
base; this attempt is recorded as blocked, not passed or a new footprint defect.

Commands from apps/backend used race detection, fts5, one-count selected cases
and the explicit temporary overlay. Cross-compiles used GOOS=linux/darwin,
GOARCH=amd64 and CGO_ENABLED=0. The normal commit hooks are required before
publication. Temporary fixtures are not added to standing test collection.

No operating process, DB, CM configuration, model or setting was changed.
Responsive component states passed; actual phone viewport geometry was not
measured by these focused component checks.

- [Plan](plan.md)
- [System design](../../specs/executors/system-design/agent-runtime-reclamation.md)


## Ownership, startup and catalog follow-up

The workspace projection resolves every distinct measured task with GetTask,
so hidden Quick Chat and automation-run owners are included. Workspace read
authorization still precedes observation; only that workspace's rows and bytes
are published. Missing ownership stays an error, not a complete empty reading.
Incomplete provider readings still remain unknown. The process reader takes
the startup lock before its existing state lock without waiting; a busy startup
returns the same unreadable marker and does not change process lifecycle.

The existing locale check reproduced 65 missing real-locale entries and 13
missing pseudo entries. Only those 13 consumed keys were translated into the
five missing real catalogs; the existing Chinese glossary/converter and pseudo
transform produced their corresponding values. The complete key check now
passes with 548 existing orphan warnings. English/Korean and unrelated catalog
values are preserved. The settings-section comment now states only its
read-only/no-draft/no-save invariant.

Disposable real-repository checks reproduced omitted hidden runtimes and a
false complete empty view for unknown ownership; startup-busy/concurrent
publication checks also reproduced the missing unreadable marker and an actual
cmd/Process data race. The candidate passed both packages under race
(task/service 1.663s; agentctl/server/process 1.114s), including authorization
before observation, foreign-workspace filtering, partial counts and incomplete
provider controls. Original physical child measurements above remain historical
and were not repeated; no new full-app or physical viewport claim is made.
