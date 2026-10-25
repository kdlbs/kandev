---
created: 2026-08-26
status: implemented
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
legacy_specs: []
---

# Implementation Plan: Server-Resolved Plugin Repository Task Creation

## Overview

Let the native task dialog create a task from a first-use plugin repository.
The backend asks the active plugin to inspect the selection, validates the
returned descriptor, and completes all provider preflight before it writes the
task. The work preserves the current dialog on desktop and phone and documents
the standardized plugin action.

## Confirmed root cause

- The plugin repository picker returns a complete descriptor for repositories
  that are not yet stored by Kandev.
- Normal REST task creation cannot set the internal trusted-descriptor marker.
  This is an intentional security boundary.
- The task service therefore passes the Bitbucket URL to the built-in parser,
  which supports only built-in providers and rejects `bitbucket.org`.
- The task row is inserted before repository resolution, so this error leaves
  a task without its selected repository.
- A pre-registered repository ID works because that path reads an authorized,
  persisted descriptor.
- The plugin action registry and the Bitbucket plugin already provide the
  foundations for a server-to-server `repositories.inspect` action.

## Scope

### In scope

- Standardize server-side plugin repository inspection.
- Validate the returned credential-free descriptor against manifest ownership
  and clone-origin rules.
- Preflight all first-use plugin selections before task or repository writes.
- Keep built-in providers, persisted repository IDs, and Host `Tasks.Create`
  behavior compatible.
- Return typed, bounded errors through task-create transports.
- Extend the in-tree plugin fixture with deterministic inspect and branch
  actions.
- Prove first-use success and failure in desktop and phone Playwright projects.
- Update public plugin-authoring and internal frontend host contracts.

### Out of scope

- Full transactionality for every task-create side effect.
- A separate repository registration interface.
- New provider-specific parsing in the task service.
- Changes to external plugin credential storage or OAuth.
- A new task-dialog layout or provider capability screen.

## Technical approach

Add a bounded `repositories.inspect` call beside the existing standardized
branch action in `internal/plugins`. It resolves the active manifest owner,
uses verified workspace context, parses the compatible direct or nested
response, and validates the complete provider descriptor.

Expose that capability to the task service through a narrow resolver seam
wired in backend composition. After task-create authorization and external-ID
deduplication, the task service resolves all first-use plugin selections into
an in-memory list. It marks only those server-returned values as trusted. The
existing trusted repository resolver then persists canonical repository rows
and task associations.

Provider resolution and validation errors occur before the task insert and use
typed categories. The current task dialog already keeps the form open and
shows a failed-create toast, so no new production UI is expected. Playwright
coverage uses the real packaged fixture and the shared dialog to prove both
responsive presentations.

## Work orders

- [done] [Task 01: Add Server Repository Inspection](task-01-add-server-repository-inspection.md)
- [done] [Task 02: Preflight Task Repository Selections](task-02-preflight-task-repository-selections.md)
- [done] [Task 03: Prove Native Desktop and Phone Flows](task-03-prove-native-desktop-and-phone-flows.md)
- [done] [Task 04: Publish the Plugin Action Contract](task-04-publish-plugin-action-contract.md)

## Dependency order

```text
Task 01 -> Task 02 -> Task 03 -> Task 04
```

The initial package is sequential. Task 02 consumes the inspection contract.
Task 03 proves the combined host and task-service behavior. Task 04 documents
the verified final shapes and limits.

## Verification strategy

- Plugin-service unit tests prove active ownership, workspace scope, action
  limits, response shapes, descriptor validation, timeout, and safe errors.
- Task-service unit tests prove external-ID ordering, request distrust,
  multi-repository preflight, no partial persistence, and compatibility paths.
- Handler tests prove stable REST and WebSocket error mapping.
- Fixture tests prove its manifest and action responses match the contract.
- Playwright proves successful first-use creation, safe failure, branch
  selection, persistence, phone reachability, and no horizontal overflow.
- Public docs, specification lint, backend race tests, frontend typecheck, and
  focused lint cover the combined change.

## Risks

- Reusing browser descriptor fields can accidentally re-open the credential
  redirect fixed in the trusted Host path. Persist only the plugin response.
- Calling inspect after task insertion recreates the empty-task bug. Pin the
  ordering with repository write-spy tests.
- A loose response parser can accept a mismatched provider or clone origin.
  Share the existing trusted descriptor validator and test malicious shapes.
- Resolving every retry before external-ID deduplication adds provider side
  effects and can turn a safe retry into a failure. Preserve dedupe ordering.
- The fixture UI already lists a plugin repository, but its backend manifest
  does not declare inspect or branch actions. Keep the fixture's UI and backend
  descriptors identical.
- A generic internal error would hide a recoverable disconnect. Preserve typed
  plugin action categories without exposing upstream response text.

## Package handoff

Implement the four work orders in order with TDD. Update each work order's
status and results after its focused checks pass. Change this plan to
`implemented` only after desktop and phone E2E, public docs validation,
specification lint, and the affected backend checks all pass.

## Results

- Server inspection validates active manifest ownership, verified workspace
  context, bounded responses, credential-free clone origin, immutable hints,
  and safe typed errors. The fixture implements the inspect and branch actions.
- Task creation preflights all first-use plugin selections before persistence,
  preserves idempotent and trusted Host paths, and maps typed errors through
  REST and WebSocket transports.
- Desktop and phone Playwright flows pass without retries. The phone flow also
  verifies native reachability, 44px submit targets, and no horizontal
  overflow.
- Public plugin contracts document the server-owned inspection boundary and
  nested response shape.
- Verification passed: affected backend race tests (3,122 tests), backend
  lint, frontend typecheck and lint, i18n checks, specification lint, public
  docs validation, diff check, formatting, and focused E2E sleep lint.
- The repository-wide E2E-sleep ESLint command remains blocked by 188 existing
  errors and 305 existing warnings in unrelated files. The changed E2E files
  pass that configuration directly, and the new-code sleep ratchet passes.
