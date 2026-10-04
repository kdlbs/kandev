---
status: active
system: tasks
created: 2026-10-04
owners:
  - kandev
---

# Task field update requirements

## Overview

Ordinary task edits must retain other accepted edits when requests overlap. Tasks owns this
contract because it owns the persisted task values and the ordinary update API; sidebar,
Kanban, agent, plugin, and Office entry points consume that contract.

The existing sidebar-edit contract defines entry-point behavior, and the parent-admission
contract defines hierarchy validity. Neither defines omission across concurrent ordinary field
updates. This is the smallest missing persistence contract, rather than another sidebar or
incident specification.

Criteria .8 through .12 define the explicit metadata-merge path.
Criteria .1 through .7 remain the previously active ordinary-field contract.

## Terms and boundary

An **ordinary update** is a request-driven partial task update. A supplied field expresses
intent; omission leaves that field alone. An explicit empty value expresses existing clear or
empty-value behavior, subject to the field's existing validation. Null currently has the same
meaning as omission for nullable request pointers, metadata, and repository inputs.

An **explicit metadata merge** supplies ordinary metadata keys through the existing merge
service path (including the task port-forwarding preference endpoint). Its keys are intent,
not a replacement document. This differs from supplying metadata on an ordinary update.

The guarantee applies between ordinary updates and against participating field-scoped writers
listed in the paired design. Intentional full-snapshot internal writes and exact/versioned
commands retain their own contracts. This does not promise that a later full-snapshot writer
preserves arbitrary earlier edits.

### REQ-TASKS-FIELD-UPDATES-001: Preserve ordinary partial edits

**Intent:** An accepted edit changes its supplied fields without restoring omitted values from
an earlier observation of the task.

#### Acceptance criteria

- **AC-TASKS-FIELD-UPDATES-001.1:** When two ordinary updates supply disjoint fields and both
  succeed, the task shall retain both edits in either commit order, including requests handled
  by independent service instances and database connections.
- **AC-TASKS-FIELD-UPDATES-001.2:** An ordinary update shall preserve the current values of
  omitted title, description, priority, state, workflow step, position, parent, human assignee,
  and metadata. Explicit empty title or description, unassignment, detachment, zero position,
  and empty repository list shall retain their current meanings. Supplying the same ordinary
  scalar field in competing requests shall leave the value of the later successful write.
- **AC-TASKS-FIELD-UPDATES-001.3:** A mixed update shall honor every supplied supported field,
  including priority with human assignment. Existing title, priority, assignee-reference,
  parent, completion, and authorization validation shall continue to apply; omitting one
  field shall not make its stale value an implicit requested change.
- **AC-TASKS-FIELD-UPDATES-001.4:** Omitted metadata shall preserve current metadata. Supplied
  metadata shall retain the existing ordinary replacement or pending-title merge behavior,
  including ordinary key deletion and null-value handling. It shall preserve current
  server-owned deferred-launch, step-handoff, handoff provenance, and Office causation records
  under their existing ownership rules. A description-only edit shall preserve the current
  title and generated-title ownership; an explicit human title edit shall resolve pending
  agent naming, and a later agent title request shall remain unable to overwrite it.
- **AC-TASKS-FIELD-UPDATES-001.5:** A field-scoped priority, state, position, or generated-title
  write committed before the ordinary update's mutation boundary shall survive when omitted
  by the ordinary update. Field-scoped metadata writes shall survive omitted metadata;
  supplied ordinary metadata remains governed by criterion .4. Existing hierarchy admission,
  normalized materialized workspace identity, and parent ABA protection shall be retained.
- **AC-TASKS-FIELD-UPDATES-001.6:** A rejected, cancelled, or failed ordinary task-row mutation
  shall leave the row unchanged by that mutation and shall emit no successful task-update or
  state-change evidence for it. Existing typed error classifications shall survive. A
  state-change event shall describe a requested change from the current state, and an omitted
  state or workflow step shall not manufacture a transition, ledger row, or entry effect.
- **AC-TASKS-FIELD-UPDATES-001.7:** Registered REST and WebSocket ordinary updates shall retain
  supplied-field presence, existing normalization, clear markers, DTOs, and error mappings.
  Responses and events shall use the existing postcommit observation contract: they may
  observe a later commit and have no new total ordering or exact receipt guarantee. Repository
  replacement remains its separate atomic operation; its failure does not roll back an
  already committed task-row edit, and suppresses the combined update's success evidence.

- **AC-TASKS-FIELD-UPDATES-001.8:** When two admitted explicit metadata merges supply disjoint
  ordinary top-level keys and both succeed, the task shall retain both intents in either
  commit order, including independent services and database connections. A merge shall apply
  supplied keys to current metadata and preserve omitted keys. Same-key competing merges
  shall apply the later successful intent under the supported value rules; there is no
  independent nested-key concurrency guarantee.
- **AC-TASKS-FIELD-UPDATES-001.9:** A metadata merge shall preserve all task scalar fields and
  associations. Between a merge and an ordinary update that omits metadata, both intents shall
  survive either commit order. Current scalar and field-scoped metadata writes committed
  before the merge shall survive when outside its supplied intent; later scalar writers
  remain field-scoped. An explicit ordinary metadata replacement or intentional full-snapshot
  writer retains its own contract, including permitted deletion or overwriting of an earlier
  merge; a later merge shall preserve that writer's current values outside its supplied keys.
- **AC-TASKS-FIELD-UPDATES-001.10:** A merge shall preserve current server-owned deferred-launch,
  step-handoff, handoff provenance, Office causation and generated-title ownership under their
  existing owner rules, including absence after an owner removes a record. It shall preserve
  materialized workspace mode/group identity. Request-owned maps shall remain unchanged.
  Nil or empty merge input shall preserve keys, with the existing successful-update timestamp
  behavior. Explicit ordinary null and nested values shall keep the supported dialect and
  pending-title semantics documented in the design; omission shall not imply null deletion.
- **AC-TASKS-FIELD-UPDATES-001.11:** A metadata-only merge shall not fabricate task state or
  workflow transitions, transition-ledger or step-entry rows, entry dispatch, runner changes,
  parent/workspace row changes, completion effects or association changes from an earlier
  observation. Existing authorization, reference admission and owner commands shall retain
  their rules. The metadata mutation and its timestamp shall commit atomically for all
  supplied keys; encoding, storage, cancellation or missing-task failure shall leave no
  partial mutation or successful update/state-change evidence, retaining typed error causes.
- **AC-TASKS-FIELD-UPDATES-001.12:** The registered port-forwarding preference endpoint shall
  use the explicit metadata-merge contract through unchanged request, response and error
  shapes. After a successful merge, responses and task-update events shall describe the same
  postcommit observation, which may include a later commit. A failed postcommit read shall
  retain the existing error and suppressed-publication behavior despite a committed mutation.
  No total event order or exact mutation receipt is introduced.

## Adjacent contracts

- [Parent admission](subtask-reparenting-drag-drop.md) owns cycle, depth, workspace, archive,
  detachment, and normalized workspace rules.
- [Repository associations](attach-workspace-sources.md) owns complete replacement and
  immutable branch-policy snapshots.
- [Human assignee](human-assignee.md), [generated titles](agent-generated-titles.md), and
  [task completion](task-completion.md) retain their existing policy.

## Out of scope

Global revisions, arbitrary merging outside the explicit merge service path, cross-request
event order, new conflict
policy for same-field edits, atomic composition with repository preparation/replacement, changes
to exact commands, Office scheduling, runner admission, launch policy, cascading lifecycle,
schema, frontend layout/copy/navigation, or new workflow fields on a transport. Existing
desktop and mobile controls receive the corrected persisted values through unchanged events.
