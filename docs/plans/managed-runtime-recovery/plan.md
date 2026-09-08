---
spec: docs/specs/agents/requirements/runtime-updates.md
decision: docs/decisions/2026-08-12-validated-managed-runtime-version-selection.md
created: 2026-08-12
updated: 2026-08-12
status: implemented
---

# Implementation Plan: Managed Runtime Version Recovery

## Overview

Turn the managed-runtime update dialog into a safe version recovery surface.
The backend will enumerate stable versions for each trusted built-in npm
package, stage and ACP-probe an exact candidate, and persist it only after
validation. The selected exact version will then drive host-local probes,
utility calls, and future standalone sessions. Desktop and mobile Settings will
support update, rollback, repair, and up-to-date outcomes without exposing
free-form package execution.

## Confirmed root cause

OpenCode's unversioned command selected `opencode-ai@1.18.16`. The wrapper
package existed, but its Linux x64 binary packages returned npm `E404`, so the
process exited before its first ACP response. The failed probe had no runtime
version, and the current update UI disabled approval. Rebuilding the unversioned
`_npx` tree selected the same incomplete release, so cache repair and a Kandev
restart could not recover the runtime.

The reproducible product failure is therefore not only an unknown-version UI
gate. It is the absence of an exact, validated, durable version choice.

## Architecture

### Trusted version foundation

- Add `internal/agent/managedruntime` as the owner of active-version records,
  stable SemVer validation using the existing Masterminds SemVer dependency,
  and the small selection-store interface.
- Persist one package-and-version system setting per built-in agent, keyed by
  trusted agent ID, so parallel changes for separate agents cannot lose each
  other through a shared JSON map and a future package-identity change cannot
  inherit an unrelated old version.
- Extend `ManagedNPMRuntimeSpec` with exact `package@version` launch,
  preparation, and deterministic execution-key helpers. Package identity and
  ACP arguments remain built-in metadata.
- Add an internal managed-runtime version option to `agents.CommandOptions`.
  The five managed agents use it only for their ACP `BuildCommand`; an empty
  option preserves current unversioned behavior.

### Host-local command routing

- Wire one active-version store through the backend composition root to the
  lifecycle manager, host utility manager, and settings controller.
- Resolve the active version immediately before standalone command building.
  Propagate context into the launch command builder where needed. Do not set an
  override for SSH or container runtimes, and preserve supported native-binary
  preference.
- Centralize host-utility inference command resolution so boot probes, manual
  refresh, model configuration, profile prompts, and sessionless prompts all
  use the same exact active version.
- Split candidate probing from capability publication. A candidate probe must
  return capabilities without replacing the live cache; the update job
  publishes them only after persistence succeeds.
- Treat saved-selection read errors as launch or probe errors. Do not silently
  fall back to an unversioned runtime when an active record exists but cannot be
  read.

### Version catalogue and transactional activation

- Resolve npm `versions` and `dist-tags.latest` for the trusted package. Parse
  strict stable SemVer, deduplicate, sort descending, and return the newest 50
  plus active/current versions.
- Add optional `target_version` to preview and a required JSON target to POST.
  Re-resolve metadata after approval and reject unpublished, prerelease, tag,
  malformed, or package-like input.
- Return backend-derived `operation` (`update`, `rollback`, `repair`, or
  `up_to_date`) and the exact preparation command.
- Stage the target under the deterministic key for `package@version`. On the
  first preparation failure, invalidate only that exact tree and retry once.
- Probe the candidate without mutating live capabilities. On success, save the
  active version and then publish capabilities. Every earlier failure leaves
  the prior active version and catalogue untouched.
- Keep existing process-local job retention, maintenance exclusion, bounded
  output, and WebSocket event names.

### Responsive Settings experience

- Keep the agent-card update icon as the desktop and mobile entry point.
- Add a shared version selector and render backend operation state. Desktop
  remains a dialog; phone remains the existing bottom drawer, with no nested
  drawer.
- Use **Update runtime**, **Roll back runtime**, **Repair runtime**, and **Up to
  date** from structural operation state. A failed job can retry its exact
  target or the operator can select another version.
- Keep one scroll owner, the safe-area footer, 44 px controls, keyboard access,
  and non-color latest/active markers. Localize all copy in every agents
  catalogue.

## API and data changes

- `RuntimeUpdateDTO`: add optional `active_version` while retaining observed
  `current_version`.
- `AgentUpdatePreviewDTO`: add `active_version`, `operation`, and
  `available_versions`; accept an optional selected target query.
- `POST /api/v1/agent-update/:agentName`: require
  `{ "target_version": "X.Y.Z" }`.
- `AgentUpdateJobDTO`: add `operation` and `active_version`; update the active
  version only at successful activation.
- Existing job statuses and WebSocket action names remain stable.

## Test strategy

- TDD the persistence, exact command generation, version filtering, operation
  classification, and targeted cache key before controller orchestration.
- Prove every normal host-utility command path and standalone lifecycle launch
  uses the active exact version, while SSH/container and no-selection paths do
  not.
- Prove candidate preparation, one exact-tree retry, failed-probe preservation,
  persistence-before-publication, restart reads, and request validation in
  backend controller/handler tests.
- TDD frontend API payloads, stale preview cancellation, selection changes,
  operation labels, disabled states, localization, and failed-target retry.
- Add desktop and `mobile-chrome` E2E coverage for rollback from an unknown
  OpenCode version and viewport containment.

## Mobile design contract

- Entry point: the existing 44 px agent-card runtime action.
- Presentation: existing `Dialog` on desktop and existing `Drawer` on phone.
- Hierarchy: active/current summary, version selector, operation explanation,
  trusted command, progress/output, fixed action footer.
- Scrolling: shared body is the only vertical scroll owner; long output wraps
  or scrolls internally without horizontal page overflow.
- Parity: desktop and mobile can select the same versions, start the same
  operation, observe progress, and recover from the same errors.

## Verification results

- Backend focused tests: 2,321 passed across managedruntime, agents,
  hostutility, lifecycle, settings controller/handlers, and backendapp.
- Backend lint: `rtk make -C apps/backend lint` — 0 issues.
- Frontend focused Vitest suite: 31 passed across 4 files.
- Frontend typecheck and lint — passed.
- i18n check and ratchet — passed; the checker retained its existing advisory
  real-locale parity warnings.
- Desktop Playwright: 6 passed. Mobile Playwright: 4 passed.
- Public documentation tests: 60 passed. Public validator: 41 pages validated.
- Fresh desktop/mobile PR screenshots were captured from synthetic fixtures,
  visually inspected, compressed, and validated against their manifest.

## Post-implementation fixup verification

- Preserved the selected target across failed preview retries and cleared stale
  failed-job state when selecting a new target.
- Kept repair distinct from up-to-date state, reused active jobs before npm
  metadata lookup, skipped redundant exact-target metadata fetches, propagated
  selection-store errors, and shared npm cache-root validation.
- Backend fixup tests: `rtk go test ./internal/agent/settings/controller
  ./internal/agent/runtime/lifecycle` — 1,705 passed.
- Frontend fixup tests: 27 passed in the runtime-operation and dialog-state
  suites.
- Fixup E2E: desktop 8 passed; mobile 4 passed.

## Implementation waves

Wave 1:

- [x] [Task 01: Build exact-version foundation](task-01-build-exact-version-foundation.md) — done

Wave 2, after Task 01:

- [x] [Task 02: Route active host runtime](task-02-route-active-host-runtime.md) — done

Wave 3, after Tasks 01 and 02:

- [x] [Task 03: Activate validated runtime versions](task-03-activate-validated-runtime-versions.md) — done

Wave 4, after Task 03:

- [x] [Task 04: Add responsive version recovery UI](task-04-add-responsive-version-recovery-ui.md) — done

Wave 5, after Task 04:

- [x] [Task 05: Prove rollback on desktop and mobile](task-05-prove-rollback-desktop-mobile.md) — done
- [x] [Task 06: Document runtime version recovery](task-06-document-runtime-version-recovery.md) — done

Execution remains in the primary conversation unless the user explicitly
authorizes implementation subagents.

## Risks and boundaries

- An ACP probe proves initialization and advertised capabilities, not every
  model/provider interaction. Authentication-required probes do not activate.
- Exact npm caches remain best-effort. Persisted selection prevents version
  drift but does not guarantee offline artifact retention.
- Avoid a second command-resolution path. Every host utility caller must use
  the shared resolver, including model configuration and profile prompts.
- Do not let request input reach package names, registry URLs, ACP arguments,
  or shell parsing.
- Do not change remote/container runtime ownership, active sessions, native-only
  installers, automatic update scheduling, or job persistence.
