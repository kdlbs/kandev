---
id: "03-supported-repair-surfaces"
title: "Expose inspect, preview, update, and readback"
status: done
wave: 3
depends_on:
  - "02-validated-registration-repair"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002
acceptance_criteria:
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.1
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.2
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.3
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.4
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.5
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.6
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.7
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.8
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.9
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.10
system_design:
  - ../../specs/workspaces/system-design/remote-repository-registration.md
---

# Task 03: Expose Inspect, Preview, Update, and Readback

## Summary

Complete #5188's supported public workflow through registered HTTP/WS routes and existing compact MCP settings tools.
Use Task 02's service validation for every transport and document the supported repair procedure.
The feature adds no rendered web control or new MCP tool.

## In scope

- Add optional `RemoteURL` pointers to the registered HTTP and WS update requests and carry them into `UpdateRepository`.
- Add `POST /api/v1/repositories/:id/remote-url/validate` and `repository.remote_url.validate`, using the shared service preview.
- Preserve existing GET/readback DTOs and map selection, scope, read-only, conflict, unavailable, and not-found outcomes consistently.
- Add repository-only `options.validate_only` to compact `update_settings_kandev` via the actual settings adapter and catalog metadata.
- Reject unknown options, non-boolean validation options, mixed preview changes, blank values, and MCP null without a write.
- Exercise the real sensitive wrapper, guarded dispatcher, registry, target authorization, service, persistence, response, and event path.
- Include active plugin fixture inspection and descriptor changes between preview and update.
- Document built-in static validation versus plugin inspection, and show inspect/preview/update/readback without SQL or origin edits.

## Out of scope

- New settings UI, mobile composition, locale strings, screenshots, dedicated MCP repository tools, and changes to task base-branch tools.
- Generic settings preview infrastructure for other domains, provider identity editing, and database workarounds.
- A claim that a successful registration repair retargets or validates an existing runtime checkout.

## Acceptance

1. Registered HTTP, WS, and guarded MCP tests perform inspect, preview, update, and persisted readback for the same registration and retain all task/source identity.
2. Invalid/mismatched/plugin-changed proposals, missing authority, foreign workspace, read-only state, blank/null values, and failed commits produce equivalent bounded outcomes without writes or success events.
3. Public documentation describes the actual supported flow and its limits. Existing base-branch tools and registration/backfill remain compatible.

## TDD and verification

First record RED for an HTTP/WS `remote_url` update that currently disappears and an MCP validation request that currently reaches mutation.
Route tests must register the real handlers and use real temporary storage.
MCP tests must use the actual guarded compact dispatcher, not a direct service call or a decoder-only assertion.
Assert the preview leaves stored URL, settings, bindings, timestamps, tasks, and event counts unchanged.

Run from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath ./internal/task/handlers -run 'Test(RepositoryRemoteURLHTTP|RepositoryRemoteURLWS|RegisteredRepositoryCheckoutDefaults)' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'Test(RepositoryRemoteURLGuardedMCP|RepositoryCheckoutDefaultsGuardedMCP|RepositoryCheckoutDefaultsExactHost|PluginRepositorySelection)' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server ./internal/mcp/handlers ./internal/settingscatalog -run 'Test.*(Settings|RepositoryRemoteURL|RepositoryDomain)' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service -run 'Test(RepositoryRemoteURL|RegisterRemoteRepository|Service_FindOrCreateRepository.*SSH)' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use causal route tests for omitted URL versus concurrent repairs and preserve existing exact Host controls.
For the API-only flow, these registered backend paths are end-to-end evidence. No browser/mobile rendering is claimed.
If implementation adds UI despite this scope, stop and reconcile the package with `/mobile-parity`, i18n, and desktop/phone rendered tests first.

Run scoped Go lint with the actual changed files, then the documentation coverage preflight described in the plan's final validation record.
Re-run only affected work-order checks after any correction.

## Files likely touched

- `apps/backend/internal/task/handlers/repository_handlers.go` and new `repository_remote_url_handlers.go`
- New `apps/backend/internal/task/handlers/repository_remote_url_test.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/backendapp/settings_domain_operations.go` and new repository-specific adapter helper/tests
- `apps/backend/internal/settingscatalog/defaults.go` and metadata tests
- Existing MCP settings contract/server tests only as needed for option forwarding and discovery
- `docs/public/git-operations.md`, `websocket-api.md`, and `automation-and-mcp.md`

## Dependencies

Task 02 supplies service validation, preview, identity fencing, URL omission preservation, and committed projection.
Task 01's parser and remote fix remain independently deliverable.

## Risks

- Ordinary MCP currently accepts the raw field. Adding HTTP passthrough alone leaves that path unvalidated.
- Silently ignored options can turn a preview into mutation. Validate repository options before entering the update branch.
- Existing HTTP/WS error helpers do not yet cover every new typed selection/conflict outcome.
- Plugin fixture tests prove host dispatch and descriptor handling, not production provider availability.
- Existing stored unsafe locators must not appear in preview, new event fields, or errors.

## Parallelism

`sequential`

## Inputs

- [Plan and current-main corrections](plan.md)
- [Registration repair requirements](../../specs/workspaces/requirements/remote-repository-registration.md#clone-url-repair)
- [Repair surfaces, validation, and persistence](../../specs/workspaces/system-design/remote-repository-registration.md#clone-url-repair)
- Existing registered checkout-default route tests, compact settings tools, settings catalog, plugin inspection fixtures, and public-doc contributor guide

## Results

Implemented on 2026-11-08.

- HTTP `POST /api/v1/repositories/:id/remote-url/validate`, HTTP `PATCH` with optional `remote_url`, WS
  `repository.remote_url.validate`, WS `repository.update` with optional `remote_url`, and compact MCP
  `options.validate_only` all reach the shared service preview or validated update.
- Error mapping covers validation, selection not-found/unavailable, read-only conflict, identity conflict, and
  not-found outcomes without echoing locators.
- Registered HTTP/WS tests, the real guarded compact dispatcher, plugin-focused service tests, and settings catalog
  metadata tests pass. Public documentation now covers the repair procedure in `git-operations.md`,
  `websocket-api.md`, and `automation-and-mcp.md`.
- No rendered control, localization string, mobile composition, screenshot, or new MCP tool was added.
- Review follow-up: `options.validate_only` now requires an actual JSON boolean and rejects unknown options at both boundaries, so `null` is rejected without a write. When the option is `false`, ordinary updates and combined repair writes are allowed. Guarded compact MCP tests cover null rejection with unchanged URL, timestamp, bindings, and zero events, plus false with an ordinary change and false with a combined repair.
