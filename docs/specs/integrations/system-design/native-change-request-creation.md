---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-NATIVE-CREATE-001
---

# Native Change-Request Creation System Design

## Purpose and boundaries

Integrations owns the provider-bound native create/partial-retry outcome. The
host-owned dialog follows [the provider mutation ADR](../../../decisions/2026-08-10-plugin-change-request-mutations.md).
GitLab's [integration design](gitlab-integration-02.md) remains authoritative
for its provider-specific creation behavior. This design changes no wire API.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-INTEGRATIONS-NATIVE-CREATE-001 | Ownership and completion |

## Components and flow

`VcsDialogsProvider` shares the native dialog among Changes repository controls,
`VcsSplitButton` menus, and session commands. The phone Changes panel uses the
same repository create control and provider. `useCreateChangeRequestHandler`
closes the submitted form immediately. `useSessionGit` / `useGitOperations`
sends built-in creation through `worktree.create_pr`; registered providers use
`createChangeRequestWithProvider`, which pushes before invoking the captured
provider target. Existing backend authorization and association remain intact.

## Ownership and completion

Each explicit opening receives a fresh opaque owner even when repository and
field values equal a previous opening. Session/task lifetime changes invalidate
previous owners, including away/back transitions. Admission captures that owner,
the raw draft, and the current creation callback and scope. The callback already
captures session, provider target, repository, and base; feedback and URL
association retain the admitted task, never the task active at completion.
While the session row is loading, the task page and Office dockview pass their
known task identity into the provider. Task resolution prefers the session's
hydrated owner, then this caller identity, then the legacy global fallback for
consumers without an explicit task. Changes to the resolved base/provider scope
also revoke form mutation authority.
Changing creation scope clears retained pushed state so a different session or
provider cannot skip its own push.

After awaiting the real operation, the handler may clear fields or publish
partial-push retry state only if the admitted owner and creation scope still
match. Replacement forms remain mounted, preserving focus and selection. Stale
results still emit truthful feedback and associate a created URL with the
admitted task/repository while the handler remains live. Existing abort behavior
on a new submission or unmount is preserved; reopening alone does not cancel
remote work.

## Failure and recovery

An owned partial failure retains the raw draft and marks its branch pushed.
Registered providers skip push only on that retained form's retry. New openings
reset pushed state. Ordinary failures/rejections retain the existing close and
reset policy. No discarded form is resurrected by a stale partial failure.

## Desktop and mobile

Composition is unchanged: desktop Changes repository create button or Git menu;
phone bottom navigation Changes tab and its repository create button. The shared
native dialog contains title, description, draft choice, Cancel and Create/Retry.
No new copy, geometry, navigation, or scroll owner is introduced. Real touch
emulation verifies the same ownership result without claiming physical-device
or native desktop OS coverage.

## Persistence, security, and observability

Draft ownership is local to a mounted provider. There is no new storage,
migration, permission, metric, cancellation protocol, or public provider schema.
Existing success/error toasts and URL association expose the original outcome.
Deferred transport integration tests and disposable-backend browser tests
provide deterministic evidence at the ownership boundary.
