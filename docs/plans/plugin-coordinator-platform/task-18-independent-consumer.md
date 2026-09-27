---
id: "18-independent-consumer"
title: "Second policy consumer and packaged compatibility contract"
status: complete
wave: 18
depends_on: ["02-capability-settings", "04-restricted-tools", "10-completion-gates", "13-automation-destination", "14-host-conversation-ui", "15-reference-plugin", "16-durable-policy", "17-reference-operations"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-012
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-012.1
  - AC-PLUGINS-COORDINATION-012.2
  - AC-PLUGINS-COORDINATION-012.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 18: Second policy consumer and packaged compatibility contract

## Summary

Prove the public surface supports another coordination style with an independently packaged observer plugin. Add a reusable two-consumer compatibility test.

## In scope

- Create ../kandev-plugin-observer as a separate template-based local repository and plugin identity. Implement proposal-first observation without automatic task adoption or starts.
- Test two consumers together, v1 compatibility, revoked grants, unsupported providers, disable/upgrade/restart, claim conflicts, and absent optional capabilities.
- Finish public capability/version tables, frontend/Go SDK examples, protocol docs, manifest requirements, and authoring recipes. Record actual minimum release only at release time.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Two different packaged policies run without adding host product branches or sharing memory, approvals, or conversations.
- The packaged contract suite proves old consumers remain compatible and missing optional capabilities degrade clearly.
- Authoring examples and SDK types describe the implemented surface and its provider/retention limits.

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

Applicable criteria: `AC-PLUGINS-COORDINATION-012.1`, `AC-PLUGINS-COORDINATION-012.2`, `AC-PLUGINS-COORDINATION-012.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd ../kandev-plugin-observer && npm ci && make test && make vet)
make -C ../kandev-plugin-observer KANDEV_SDK="$(pwd)/apps/backend" verify-package-host
(cd apps/backend && go test ./internal/plugins/... ./pkg/pluginsdk/...)
(cd apps && pnpm --filter @kandev/plugin-sdk test && pnpm --filter @kandev/plugin-sdk typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/coordinator-compatibility.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-coordinator-compatibility.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Evidence to create:

- `../kandev-plugin-observer/server/observer_test.go`: `TestObserverPolicy`.
- `apps/web/e2e/tests/plugins/coordinator-compatibility.spec.ts`: `two independent packages and lifecycle contract`.
- `apps/web/e2e/tests/plugins/mobile-coordinator-compatibility.spec.ts`: `phone policy parity and unavailable capability`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `../kandev-plugin-observer/ (new dedicated repository)`
- `apps/web/e2e/tests/plugins/coordinator-compatibility.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-coordinator-compatibility.spec.ts`
- `docs/public/plugins-authoring.md`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/plans/plugins/GRPC-CONTRACT.md`
- `apps/packages/plugin-sdk/`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 02](task-02-capability-settings.md)
- [Task 04](task-04-restricted-tools.md)
- [Task 10](task-10-completion-gates.md)
- [Task 13](task-13-automation-destination.md)
- [Task 14](task-14-host-conversation-ui.md)
- [Task 15](task-15-reference-plugin.md)
- [Task 16](task-16-durable-policy.md)
- [Task 17](task-17-reference-operations.md)

## Risks

A second instance of the same policy does not prove extensibility. Use a distinct package, tool set, and policy with independent persisted state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

WO18 is complete. A separately packaged observer proposes follow-up work and waits
for human approval; it has a distinct manifest, policy, and persisted state from the
coordinator. Observer tests/vet and package validation passed. Desktop compatibility
E2E verified both packaged consumers, the v1 fixture, lifecycle recovery, and grant
revocation; phone compatibility E2E verified policy parity and unavailable
capability handling. Public authoring documentation and SDK/protocol references were
validated. No managed provider is advertised because a real adapter launch/resume
smoke matrix has not been completed. No remote repository was created or published.
