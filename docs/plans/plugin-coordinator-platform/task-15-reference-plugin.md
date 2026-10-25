---
id: "15-reference-plugin"
title: "Packaged reference coordinator and named instances"
status: complete
wave: 15
depends_on: ["04-restricted-tools", "05-durable-input", "06-workspace-observations", "07-task-commands", "08-execution-controls", "09-task-claims", "14-host-conversation-ui"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-010
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-010.1
  - AC-PLUGINS-MANAGED-COORDINATION-010.2
  - AC-PLUGINS-MANAGED-COORDINATION-010.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 15: Packaged reference coordinator and named instances

## Summary

Build the first useful coordinator in a dedicated local plugin repository. It owns roles, named instances, memory, chat composition, and delegated-task follow-up.

## In scope

- Start a sibling checkout from the official template, recording template and SDK revisions. Use ../kandev-plugin-coordinator unless that path already contains unrelated work; never overwrite it.
- Implement role/instance configuration, namespaced memory, declared managed tools, public Host delegation, and canonical task status. Compose Host chat in the desktop/phone product route.
- Package and install into an isolated test instance. Add external plugin tests and host fixture contract tests; keep production plugin code out of the monorepo.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A packaged plugin supports two independent named instances with different profiles/instructions and isolated memory.
- An instance delegates a task, receives a follow-up, and survives disable/upgrade without private Host access.
- Desktop and phone expose chat, tasks, instance configuration, and pause using the approved previews.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-02: Workspace conversation

```text
Desktop: plugin navigation > Coordinator
+---------------------------------------------------------------------+
| Product workspace   [Delivery lead v] [Settings] [Pause]              |
| Outcomes: 8 completed  1 blocked  Usage: USD 2.10 measured             |
+----------------------------------+----------------------------------+
| Chat                             | Tasks  [Search] [Filters]        |
| You: follow up the review        | Blocked                          |
| Agent: proposal ready            | [Task A] Waiting for answer      |
| [Proposal: Review fix] [Approve] | Running                          |
|                                  | [Task B] Implementing            |
| Input queued: 1  [Cancel]         | Completed                        |
| [Message composer]        [Send] | [Task C] Verified                |
+----------------------------------+----------------------------------+
Phone: plugin navigation > Coordinator
+------------------------------+
| < Product [Delivery lead v] : |
| [Chat] [Tasks] [Outcomes]     |
+------------------------------+
| Chat messages                |
| Proposal: Review fix         |
| [Inspect] [Approve]          |
| Input queued: 1 [Cancel]     |
+------------------------------+
| [Message]             [Send] |
+------------------------------+
Phone instance/filter selection: bottom drawer
+------------------------------+
| Choose instance       [Done] |
| (o) Delivery lead            |
| ( ) Reviewer                 |
| [Add instance]               |
+------------------------------+
```

Desktop split composition and phone tabs are required; widths and wording are illustrative. Header/composer are pinned, only the active tab body scrolls, and selectors use bottom drawers. Empty: create an instance. Loading: retain the shell. Disconnected: retain draft and retry identity. Paused: show retained input count and Resume. Revoked/unsupported: explain the blocked capability and link to host settings. Pending human interactions use native shared controls.

### UI-05: Instance settings and proposals

```text
Desktop: Coordinator > Settings / proposal detail
+---------------------------------------------------------------+
| Instance: Delivery lead   Role: [Chief of staff v]              |
| Profile: [Supported agent v]  Executor: [Local v]               |
| Instructions: [User-defined coordination rules...]             |
| Scope: [Selected tasks v]   Budget: [USD 10 estimated]          |
| [Pause instance]                                    [Save]    |
| Proposal: Review fix  Revision 3   [Inspect] [Approve] [Reject] |
+---------------------------------------------------------------+
Phone: instance settings, separate full-height view
+------------------------------+
| < Delivery lead       [Save]|
| [Role v]                    |
| [Supported profile v]       |
| [Executor v]                |
| Instructions                |
| [User rules...]             |
| [Task scope v]              |
| [Budget and limits]         |
| [Pause instance]            |
+------------------------------+
```

Phone proposal detail is a full-height view with fixed Approve/Reject actions. Role/profile/executor/scope selection uses drawers. Unsupported profiles show a reason. Duplicate approval reads one receipt; stale proposals require inspection of the new revision. Pause preserves memory and pending inputs.

Applicable criteria: `AC-PLUGINS-MANAGED-COORDINATION-010.1`, `AC-PLUGINS-MANAGED-COORDINATION-010.2`, `AC-PLUGINS-MANAGED-COORDINATION-010.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd ../kandev-plugin-coordinator && npm ci && make test && make vet)
make -C ../kandev-plugin-coordinator KANDEV_SDK="$(pwd)/apps/backend" verify-package-host
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/reference-coordinator.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-reference-coordinator.spec.ts)
```

Evidence to create:

- `../kandev-plugin-coordinator/server/instances_test.go`: `TestCoordinatorInstances`.
- `apps/web/e2e/tests/plugins/reference-coordinator.spec.ts`: `packaged instance creation and delegation`.
- `apps/web/e2e/tests/plugins/mobile-reference-coordinator.spec.ts`: `phone instance selection and settings`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `../kandev-plugin-coordinator/manifest.yaml (new dedicated repository)`
- `../kandev-plugin-coordinator/server/`
- `../kandev-plugin-coordinator/ui/`
- `../kandev-plugin-coordinator/Makefile`
- `apps/web/e2e/tests/plugins/reference-coordinator.spec.ts (new package smoke)`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 04](task-04-restricted-tools.md)
- [Task 05](task-05-durable-input.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)
- [Task 08](task-08-execution-controls.md)
- [Task 09](task-09-task-claims.md)
- [Task 14](task-14-host-conversation-ui.md)

## Risks

The template is a starting point, not a production coordinator. Retain its working package/test targets while adapting source layout. Repository publication requires a separate authorized delivery step.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Implementation notes

The new package-smoke fixture reads ../kandev-plugin-coordinator relative to the host root, discovers the archive from its manifest ID/version, and installs it into isolated E2E data. Fail clearly if the checkout or package is absent; do not silently substitute the in-tree fixture.

## Results

WO15 is complete. The template-based coordinator package implements independently
configured instances, isolated memory, managed chat, delegation, follow-up, and
pause/restart behavior through public Host APIs. The packaged desktop and phone
flows passed, including instance configuration and task delegation. The coordinator
unit/recipe tests, `make vet`, and host-package validation passed. Its checkout is
`../kandev-plugin-coordinator-template-main` and the compatibility path
`../kandev-plugin-coordinator` points to that same local checkout. No remote
repository was created or published.

### Review remediation (2026-09-27)

The packaged coordinator UI now keeps uncertain retries attached to the original
instance across selection changes and returns them to that instance with the same
request and idempotency identity. Deferred status/input responses are ignored when
stale. Packaged coordinator desktop and phone E2E regressions passed after rebuilding
the host web bundle and plugin archive; coordinator package host validation passed.
