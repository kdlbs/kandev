---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUN-OBSERVATION-001
  - REQ-OFFICE-DASHBOARD-001
---

# Office Run and Activity Observation

## Mapping and ownership

Office dashboard projections own labels. REQ-OFFICE-RUN-OBSERVATION-001.1/.2
map to run projection below; .3/.4 and REQ-OFFICE-DASHBOARD-001.7/.8 map to
activity projection; .5 maps to presentation. Existing IDs remain API/navigation
keys. All new fields are additive.

## Run projection

`dashboard/run_detail.go:buildInvocation` currently assigns `agent.ID` to adapter.
Replace that assignment with actual invocation evidence: the exact run-session
record for taskless execution, or recorded invocation/route/task-session runtime
identity for task-bound runs. A configured profile may be displayed separately
as configured, never asserted as historical invocation. Missing evidence returns
empty actual fields, rendered as localized unavailable/not-started state.
Do not infer historical adapter from a profile that may since have changed.

Extend run skill snapshots with captured display name and slug alongside existing
skill ID/version/hash. `scheduler_integration.go` snapshot construction captures
these from the resolved manifest/skill records; `dashboard/dto.go` projects them.
For old snapshots, resolve current workspace-visible skill metadata in a bounded
batch, mark the label source current, or show a localized missing-skill label.
Never mutate historic content hashes or pretend a backfilled name was captured.

Expose the Office agent's display name separately from adapter and runtime profile.
Update `office-runs-api.ts`, agent run header and `runtime-panel.tsx`; keep technical
IDs secondary and copyable where the existing diagnostics surface supports it.

## Activity projection

Add optional `actor_name`, `target_name`, `target_identifier` fields to the
activity response projection (not a rewrite of append-only activity rows).
Use one workspace-scoped, bounded batch resolver for activity lists, target
activity and dashboard recent activity. Agent targets use names; tasks prefer
identifier plus title; runs retain a readable Run label and short ID. Resolve
only authorized entities in the entry's workspace. Service actors such as
`office-scheduler` use localized client labels. Missing rows retain stable IDs
and localized fallbacks. No request per rendered row and no dependence on the
sidebar having loaded all agents/tasks.

Wire additive fields through `office-activity-normalize.ts` and Office types.
`activity-row.tsx` uses display names for initials and labels, retains IDs for
links, and applies the same resolver result in special-action and generic rows.
Names are current entity names; this does not introduce historical actor-name
snapshots. Read failures do not erase the existing activity feed.

## Presentation and validation

Existing run detail and activity layouts remain. On phones, labels wrap within
min-width-zero containers; diagnostics cannot force horizontal document scroll.
Use localized fallback text in all five shipping locales, regenerate Traditional
Chinese with the repository script, and preserve user names verbatim.

Unit/API tests cover new, legacy, deleted, renamed and cross-workspace entities,
actual versus configured adapters, routed fallback and bounded query count.
Desktop/mobile E2E cover direct navigation without preloaded sidebar data and
long names. Runtime tests retain version/hash assertions. No authorization or
persistence migration is required for activity labels; skill snapshot columns
are additive with empty defaults for old rows.
