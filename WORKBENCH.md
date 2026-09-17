# Private Kandev orchestration workbench

Review branch: [feat/workspace-orchestration](https://github.com/Corey-Fogg/kandev-orchestration/tree/feat/workspace-orchestration).
Compare all changes: [v0.94.0 base → workbench](https://github.com/Corey-Fogg/kandev-orchestration/compare/main...feat/workspace-orchestration).

This private repository preserves Kandev's public history through **v0.94.0**
(`bf819a0228e742d069c528293d848c985a4d1bd1`) and imports the existing custom
orchestration prototype above it. `main` stays at that release for review. The
default feature branch contains the code, complete scope audit and remaining-work package.
Private local setup notes and identifying examples have been replaced with
generic material; original local snapshot/backup refs are not published.

## Start here

| Review question | Document |
| --- | --- |
| What is all the remaining work, and in what order? | [Delivery plan and milestones](docs/plans/orchestration-delivery/plan.md) |
| What already exists, including scope beyond the original issue? | [Complete scope audit](docs/review/orchestration/scope-audit.md) and [476-path prototype inventory](docs/review/orchestration/scope-file-inventory.csv) |
| What will the central task view do? | [Coordinator requirements/design](docs/specs/orchestration/README.md) and [three view work orders](docs/plans/workspace-coordinator-view/plan.md) |
| What remains in the larger personal assistant? | [Assistant requirements](docs/specs/orchestration/requirements/personal-assistant.md), [design](docs/specs/orchestration/system-design/personal-assistant.md), [implementation work orders](docs/plans/personal-assistant/plan.md) |
| What can we learn from the related plugin? | [Pinned source review and compatibility limits](docs/review/orchestration/plugin-review.md) |
| How do we run it in our actual installation? | [Candidate, migration and rollback runbook](docs/plans/orchestration-delivery/dogfood-runbook.md) |
| What evidence and demo already exist? | [Review packet](docs/review/orchestration/README.md) |

## Current readiness

| Area | State |
| --- | --- |
| Roles, workspace assignments, persistent chat, task delegation/callbacks | Implemented prototype, rebased and validated against v0.94.0 |
| Optional Automation delivery to a coordinator | Implemented; portable YAML/ZIP export of this destination explicitly unsupported |
| Assistant ownership/intake/objectives | Backend implemented; retained privacy guards must remain active |
| Assistant memory/context | Partial; validation metadata and dedicated pagination/scope coverage remain |
| Central workspace task view beside chat | Fully planned; implementation pending |
| Assistant capability enforcement, attention, resolution, UI and workspace grants | Fully planned; implementation pending |
| Independent assistant toggle | Planned prerequisite for coordinator-only dogfooding |
| Actual live-data rehearsal and deployment | Pending; live service unchanged |

## Development and contribution

Use normal focused commits/topic branches from the private integration branch and
the repository's checks/hooks. `origin` targets this private repository. Local
`upstream` fetches public Kandev and has pushes disabled. Inherited GitHub Actions
are disabled because they include publishing/review integrations; optional private
validation CI has its own work order.

Keep implementation commits separate from private operational/review records.
For upstream, create a fresh branch in the public fork from the agreed base and
cherry-pick/export only the reviewed feature slices. Preserve required migrations
and ownership guards. Do not push this entire private branch or backup refs to
the public fork. See [upstream export](docs/plans/orchestration-delivery/task-06-upstream-export.md).

Yes, this workbench can supply the build used in actual Kandev. Source publication
does not change the running application: qualify a complete versioned bundle,
rehearse migration/rollback against a private copy, then perform a reviewed live
cutover. The first pilot enables the coordinator with the unfinished assistant off.
Screenshots/video always come from separate fictional fixtures with generic prompts.
