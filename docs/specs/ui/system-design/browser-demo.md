---
status: current
system: ui
requirements:
  - REQ-UI-BROWSER-DEMO-001
---

# Browser demo design

## Ownership and isolation

The UI owns the demo transport and fixtures under `apps/web/lib/browser-demo`.
The backend and real agent adapters remain unchanged.
`mode.ts` limits installation to the dedicated build or explicit development entry point.
`src/main.tsx` waits for demo installation before it loads the application boot payload.

## Transport and state

`install.ts` replaces browser fetch and WebSocket transport for demo requests.
The worker entry point dispatches transport messages to bounded HTTP and WebSocket modules.
`runtime-state.ts` owns shared state. Plan, workspace, Git, terminal, and agent modules own their operations.
Each request returns a correlated response, including handler failures.
Worker errors reject pending requests and produce the translated application boot error when initialization fails.
Session storage retains demo changes within each tab. Reload preserves that tab’s snapshot; a new tab starts an independent demo.
Reset clears only the current tab’s demo snapshot.
Snapshots include task-specific file contents and edited plans.
Workspace seeds use each task's selected repository, including a single API repository.
Denied storage access and write quotas leave the current demo usable without persistence.
Unsupported routes remain explicit errors rather than false success responses.

## Application capabilities

`scenario.ts` supplies repositories, sessions, histories, approval requests, questions, plans, and pull-request comments.
The file fixtures expose repository-specific trees and changed-file contents.
The worker simulates task progress through tool events and an idle review state.
Workflow modules provide editable workflow data, templates, synchronization, and transfer operations.
Clarification responses update the seeded question and publish the resolved message and session state.
Workflow imports validate complete step and event shapes before state changes.
Simulated turns follow the current step's completion move action, including imported and template workflows.
Move responses use current workflow steps. Bulk moves validate their destination and publish each task update.
The system runtime supplies database, disk, and storage data through the current typed contracts.
These modules implement criteria .2 through .7 without a separate UI.

The discovery runtime handles repository discovery and refresh through the current selector contracts.
The Jira runtime supplies local projects, statuses, tickets, dashboard filters, and transitions.
Follow-up sends preserve history and emit ordered thinking, tool, and answer events through the conversation transport.
Replies vary between turns. Queued sends drain locally, and retries retain their original message identity.
Each session serializes its simulated turns and returns to idle review.
Stop, task deletion, and reset cancel the session’s delayed messages and queued turns.
Reload settles saved running or starting sessions to idle without replaying an incomplete turn.
Workflow and step deletion remove matching tasks, sessions, and messages from the worker’s canonical state.
These responses remain simulations and do not use a model or integration credentials. Criterion .9 covers follow-up behavior.

## Release distribution

`scripts/browser-demo/build-web-demo.sh` builds the SPA under `/browser-demo/app/`.
Relative output paths resolve from the repository root. Absolute paths remain unchanged.
The package command writes to `dist-browser-demo`, independently of the normal application build.
The Stable release workflow packages the bundle, enforces the 25 MiB compressed limit, and publishes its SHA-256 checksum.
Required artifact uploads allow three attempts, with 30-second and 60-second waits.
Nightly builds omit the demo. Backfills of tags without its build script omit its artifact and landing dispatch.
New Stable releases require the build script. Supported backfill tags retain all required demo checks.
After successful Stable publication, the release workflow sends `kandev_release` to the landing repository with the exact release tag.
The dispatch job requires `LANDING_REPOSITORY_DISPATCH_TOKEN` in the `release` environment, with Contents write access to `kdlbs/landing`.
The landing repository consumes this archive separately. Criteria .1 and .8 cover installation and distribution.

## Verification

Worker, scenario, workflow, system-runtime, and installation tests exercise the simulated protocol and seed data.
Typecheck catches changes to shared response contracts. Translation checks cover the development entry-point action.
The production build checks worker bundling and the public base path.
Local browser inspection remains necessary to confirm rendered interactions across desktop and phone layouts.
