# Workspace orchestrators implementation record

Status: coordinator foundation implemented. This sanitized record preserves
technical scope and validation; installation details and conversation-specific
history remain outside the repository.

## Delivered behavior

1. Reusable global roles own identity, icons and behavioral instructions.
   Workspace assignments own selected execution profile/executor and local context.
   Role edits are resolved on each turn without copying conversation history.
2. A workspace supports multiple coordinator assignments and persistent native
   conversations, with list/configuration/sidebar/mobile entry points and return
   links from ordinary tasks.
3. Coordinators delegate through canonical Kanban tasks and existing execution
   profiles. Adoption preserves worker assignment; explicit reassignment remains
   supported. Scope checks reject foreign resources and silent account fallback.
4. Task ownership routes lifecycle callbacks to the appropriate coordinator.
   Stable callback identities deduplicate transitions and retain separate results.
   Review and blockers remain distinct from task completion.
5. Orchestration owns its registry, persona instructions/memory, conversation
   mapping, runtime, HTTP/CLI APIs and locale namespace. It shares core tasks,
   sessions, comments, authentication, run dispatch, execution and chat rendering.
   Its backend dependency closure excludes Office.
6. Feature gating is independent of Office and defaults off. Disabled/paused
   entry points do not launch new runs; saved state remains. Office compatibility
   routes reject registered coordinator personas and conversations.
7. One-time migrations preserve identity/history and prevent legacy writes from
   repopulating migrated memory. Role migration preserves divergent assignment
   configurations. Retained conversation ownership protects private history.
8. Per-turn/session credentials, explicit retry and restart interruption handling
   preserve authority without automatically replaying uncertain external writes.

## Global role and conversation validation

Repository/runtime/handler tests cover role propagation, migration replay,
scope and selected-account retention, retry credentials, finished-run rejection,
bounded context, callback delivery and review/reopen gates. Frontend tests cover
shared identity, routes, settings save behavior and task links. Browser scenarios
cover multiple assignments, roles/icons, conversation turns/callbacks, feature
flag combinations and desktop/mobile navigation.

The local preview checks were historical installation evidence, not a rehearsal
of the current production database. Real provider subscription identity and
model judgment are not established by mock tests.

## Automation destination extension

Core Automations can deliver scheduled or manual prompts to an assignment's
existing conversation. Role/profile/executor/context are inherited; task-only
overrides are rejected or cleared. Stable firing keys deduplicate admission.
A dispatch audit does not own the conversation; history retention cannot delete
the shared chat. Accepted delivery is distinct from completed work, and interrupted
delivery remains uncertain rather than replayed.

Tests cover validation, round trips, inherited identity, deduplication, failures,
retention and the complete synthetic conversation delivery flow. No recurring
production job was created by the prototype's tests.

At v0.94.0, YAML/ZIP export cannot represent a coordinator destination and returns
an explicit unsupported-target error rather than changing its meaning.

## v0.94.0 integration and remaining scope

The complete local implementation was integrated onto exact v0.94.0, with affected
Go suites, 206 frontend tests, type checking, database conformance/race checks,
both coordinator browser specs together, fresh builds and normal commit hooks
passing at the recorded baseline. See the
[scope audit](../../review/orchestration/scope-audit.md).

The linked-task configuration list is bounded and is not the newly planned
central task view. See [its design package](../workspace-coordinator-view/plan.md)
and the [remaining delivery plan](../orchestration-delivery/plan.md). The larger
personal-assistant extension remains partial. Worker-message transport receipts,
complete attention reconciliation and enforced read-only provider execution must
not be inferred from instruction guidance alone.
