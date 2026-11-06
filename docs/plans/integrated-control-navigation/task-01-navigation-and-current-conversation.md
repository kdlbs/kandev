---
id: integrated-control-navigation-task-01
title: Navigation and current conversation
status: in-progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-INTEGRATED-CONTROL-001
acceptance_criteria:
  - AC-UI-INTEGRATED-CONTROL-001.1
  - AC-UI-INTEGRATED-CONTROL-001.2
  - AC-UI-INTEGRATED-CONTROL-001.3
  - AC-UI-INTEGRATED-CONTROL-001.4
system_design:
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Navigation and current conversation work order

Own only the isolated integrated-control worktree. Preserve the original feature
requirements in the linked UI requirement/design. Implement the working vertical
flow, not a global page containing workspace links. Use original IDs and existing
native authorization. Never call conversation creation/preparation/resumption
from selection; do not clear drafts or change real workspace selection.

Deliver native typed flag wiring, shared desktop/phone navigation, real current
data, an original-conversation adapter, and actual focused verification results.
Add permanent regressions for navigation admission, ownership, asynchronous read
lifetimes, original VCS scope, and desktop/phone user outcomes. Do not add new
pollers, fingerprinting, database cleanup, navigation-triggered agent launches,
or changes to provider/model ordering. The delivery reviewer owns integration.

Acceptance: OFF has the legacy picker/sidebar and no integrated requests;
ON opens five-menu dedicated navigation; selecting a second workspace's existing
current conversation keeps the Integrated control context, resolves the original
task/session, preserves a draft across selection, and starts no execution or
conversation POST. Unknown, failed, denied, missing-task and missing-session
outcomes are visible without creating replacements. The phone drawer uses the
same navigation tree. Scope limitations are retained as original remaining work.

## Read-only automatic recovery follow-up

The shared resumption focus listener omitted skipAutomaticRecovery. A disposable
copy of the existing lifecycle harness reproduced status/resume/restore calls
while inspecting a read-only conversation, and a pending startup response still
resuming after inspection became read-only. Focus now uses the same guard and
effect dependency; changing the flag invalidates the existing committed request
generation before pending automatic responses can dispatch a resume.

All 46 existing cases and both disposable cases passed together (48 GREEN,
vitest 3.71s), retaining explicit manual retry coverage. The first disposable
copy had unresolved relative imports and ran zero tests; correcting temporary
aliases produced the actual two REDs and GREEN. No standing test was added.

This atomic candidate starts at public PR4515 head5f74fd4083736392ddb762ce7b915b4796c1bef0
in the already-existing control worktree. The original conversation-history
worktree, localbafc39 and its local follow-upa8f435 remain preserved. Product
hook, its existing test, vitest config and lockfile have no diff between5f74 and
bafc39, so the same 48-case evidence applies to these identical three product
lines. Documentation here uses only files present in the public parent, without
pulling in unpublished conversation-index/history work. Final normal hooks and
independent review remain required before public delivery; no push occurred.


## Review feedback follow-up

The public-parent follow-up marks all three control destination/conversation
link groups with the required pointer class and uses alert regions for the three
collection/partial-data errors. Loading and disabled-feature statuses stay polite.
The separate functional follow-up formats the activity timestamp through the
existing active-locale formatter while preserving the machine dateTime value.
Scoped PR reads now fence the committed key in a layout effect and still read
associations after a rejected sync, preserving the original rejection.

A disposable mounted React/Suspense probe reproduced both functional PR-read
failures against the exact pre-fix module (two REDs, three ownership mismatches
already GREEN). All five cases passed after the fixes (1.41s total); the existing
locale formatter suite passed all 40 cases (0.995s total). An initial probe read
an abandoned test render rather than a committed result; its layout-only observer
was corrected before repeating both baseline and corrected-product executions.
No standing tests or runtime operations were added. The disposable probes do not satisfy the permanent-coverage review finding.
Permanent real-module regressions are now part of this work order. Current-head
hosted verification and final review disposition remain pending.

## Delivery review regression coverage

Permanent tests now cover route admission, original task/workspace/session context,
all four task ownership predicates, commit-time ownership changes, authorization
generations, stale and unmounted reads, scoped PR filtering and WebSocket events,
unlink/read races, rejected sync refresh, abandoned renders, and inspection-only
focus/visibility and pending recovery invalidation with explicit retry controls.
Hosted RED/GREEN results and desktop/phone acceptance are pending; no local
contributor code, tests, compiler, hooks, or dependency installation was executed.

The delivery correction also keeps Azure embedded reads and the route's workspace
collection local, invalidates retained callbacks after unmount, and masks task
ownership across workspace/coordinator/authentication changes before commit.
Permanent component tests exercise the native renderer boundary and Azure link
with the real context/store, and existing merge-button tests verify explicit
GitHub/GitLab actions use the original workspace. The flag test covers all three
profile defaults and restart metadata; activity rendering checks locale changes.
Permanent desktop and phone browser cases cover opt-in entry, all five destinations,
the original second-workspace conversation, draft/runtime selection, visible failure
states, no execution, global selection/cache preservation, and phone hit targets,
containment, scroll ownership, dismissal and back navigation. These cases have
been authored; actual hosted results remain pending.
