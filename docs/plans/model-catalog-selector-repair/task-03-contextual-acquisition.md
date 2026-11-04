---
id: "03-contextual-acquisition"
title: "Acquire and publish in the owning context"
status: pending
wave: 3
depends_on:
  - "02-fusion-bindings"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.4
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 03: Acquire and publish in the owning context

## Summary

Acquire native relationship evidence within the same live or probe context as ACP.
Then enrich every model catalog through Task 02's normalizer.
Keep acquisition bounded and optional. Preserve current option, cache, restoration, and mutation authority.

## In scope

- Add `internal/agentctl/devincatalog` with an immutable prepared snapshot and an injected owner runner.
- Carry an optional internal source interface through `adapter.Config`, `ToSharedConfig`, and `shared.Config`.
- Prepare live evidence once after successful initialization/authentication, outside adapter/configuration locks.
- Supply the final admitted live binary, environment, working directory, wrapper, and generation from `process.Manager`.
- Use `prepareSessionArgs` and manager-owned process operations for live listing.
- Use `prepareACPLaunch`, the sanitized probe environment, and existing ACP process-group cleanup for settings listing.
- Acquire at most once after each admitted uncached probe. Keep existing hostutility caching and flight behavior.
- Reject unknown command/flag shapes rather than remove wrappers or use another binary/account.
- Preserve separate stdout/stderr, explicit overflow, cancellation, process-tree cleanup, and bounded sanitized outcome logs.
- Route every normalized model catalog through pure enrichment before caching or publication.
- Fence source publication by launch/context generation and discard evidence on stop, replacement, or reconfiguration.

## Out of scope

- A host-global native cache, periodic listing job, new public endpoint, browser DTO, or database schema.
- RPC or subprocess work inside dialect hooks or notification conversion.
- Replacing profile resolution, live busy-turn rules, partial application, or conversation identity.
- Extending host CLI suggestions or acquiring account credentials.

## Acceptance

1. Both real live/probe paths publish the same qualified composition while preserving the exact ACP choices and authoritative Thinking/Speed snapshots.
2. The listing uses the matching launch context, stays within its byte/time budget, reaps descendants, and cannot publish into another generation.
3. Every failure preserves usable ordinary selection, and renders/cache hits/notifications spawn no listing process.

## Implementation boundaries

Use a source with two phases: bounded preparation and pure enrichment.
The owner supplies a runner from an immutable launch snapshot. The source never discovers an ambient host binary itself.
Only the registered Devin ACP path receives the production source.
Other transports and providers retain a nil source and unchanged behavior.

The live runner uses `BeginOwnedOperation` and `StartPipedProcess`, with cleanup through the current manager process API.
The utility runner reuses `configureACPCommand` and `cleanupACPCommand` lifecycle handling.
Add bounded writer/runner support where required. Existing unbounded or silently truncating output helpers cannot establish complete JSON.
Do not broaden generic process behavior unrelated to the source.

The source budget is 10 seconds, 2 MiB stdout, and 64 KiB stderr, inside the parent's remaining deadline.
On overflow, cancel the command, drain or close owned pipes, and reap the complete process tree.
Reject trailing JSON data and a complete JSON prefix followed by discarded bytes.
Output larger than 64 KiB but within budget must succeed.

The adapter's common enrichment helper covers:

- Typed and `_meta.configOptions` initial snapshots.
- New, load, resume, and reset snapshots and synchronous model-state reads.
- `ConfigOptionUpdate` notifications.
- Model and configuration-option response/convergence snapshots.

Apply enrichment before assigning `availableConfigOptions` or publishing events.
Keep the current session, execution, configuration generations, and settings policies intact.
Pure enrichment re-intersects with each current catalog, so withdrawn values disappear immediately.
Do not mutate shared cached catalog pointers.

Settings results use the existing hostutility scope and opaque context revision.
A canceled browser waiter retains the existing shared-flight policy for other waiters.
No additional cross-profile source flight or cache exists.
A live source belongs to its captured launch generation and receives cancellation from that owner.
Initialization failure or source timeout leaves ordinary selection until an explicit refresh or new initialization retries.

## TDD cases

Create the source and integration regressions before implementation.

- `TestDevinCatalogSourceBounds`: valid JSON above 64 KiB, stdout/stderr separation, both overflow limits, malformed/trailing/truncated output, and bounded parsing.
- `TestDevinCatalogContextParity`: fake child evidence for executable, wrapper, configuration/authentication markers, proxy, workdir, and executor context.
- `TestDevinCatalogCancellation`: blocked output and surviving descendants terminate on timeout, cancellation, and manager stop.
- `TestDevinCatalogGenerationFence`: deferred old result cannot enrich a replacement or reconfigured launch.
- `TestDevinCatalogPublicationBoundaries`: each live snapshot boundary above retains compositions, unbound leaves, exact values, and current-model state.
- `TestDevinCompositionKeepsProviderOptions`: an effort-like model ID disagrees with returned Thinking, and a later snapshot removes Speed. Only returned options remain authoritative.
- `TestDevinCatalogProbeRoundTrip`: real probe/native subprocess-shaped fixture reaches hostutility projection and cached results without native-only availability.
- `TestDevinCatalogDoesNotRelistOnUpdates`: repeated updates and normalized result cache hits invoke no new listing.
- `TestDevinCatalogUnsupportedContext`: unsupported command/flags, missing wrapper, and missing executable preserve ordinary ACP data without another launch attempt.
- `TestDevinCatalogDiagnosticsSanitized`: sensitive output and launch markers do not reach listing diagnostics or generic process logs.

Use synthetic environment markers, not real credentials, in process tests.
Keep environment/command construction tests platform-neutral. Gate only process semantics that require a specific OS.

## Verification

Run from the repository root after Task 02's real fixture gate.
All new integration tests use names containing `Catalog`, `Devin`, or `Composition` so the focused commands discover them.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/modelcatalog ./internal/agentctl/acpcompat ./internal/agentctl/devincatalog ./internal/agentctl/server/adapter ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility ./internal/agentctl/server/process -run 'Test.*(Catalog|Devin|Composition|Config.*Shared)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/devincatalog ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'TestDevinCatalog.*(Cancellation|Generation|Bounds|Context)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility ./internal/agent/runtime/lifecycle -run 'Test.*(Catalog|ModelConfig|Profile.*(Context|Probe))' -count=1)
(cd apps/backend && go build -trimpath ./cmd/agentctl ./cmd/kandev)
git ls-files --modified --others --exclude-standard -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

Inspect discovered test names before recording success. An empty selected suite is not evidence.
Record actual commands and results, including process cleanup and race results.
Add any new test name outside these patterns to an explicit command.

## Files likely touched

- New `apps/backend/internal/agentctl/devincatalog/source.go`, `source_test.go`, `command.go`, and `command_test.go`
- New `apps/backend/internal/agentctl/server/process/devin_catalog.go` and `devin_catalog_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go` and existing process ownership helpers as required
- New `apps/backend/internal/agentctl/server/utility/devin_catalog.go` and `devin_catalog_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go` and `acp_command.go`
- `apps/backend/internal/agentctl/server/adapter/adapter.go` and `adapter_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/config.go`
- New `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_catalog_enrichment.go` and `adapter_catalog_enrichment_test.go`
- Existing ACP `adapter.go`, `adapter_session.go`, `adapter_updates.go`, and `meta_convert.go` only at the required normalization boundaries
- Hostutility catalog/cache round-trip tests and lifecycle catalog projection tests

## Dependencies

Task 02's captured source qualification and pure normalizer.
The original Task 04 remains incomplete until this integration also passes.

## Risks

- Re-reading ambient environment or resolving `devin` again can select another account or executable.
- Performing acquisition under a configuration lock can block notification processing or teardown.
- Some publication paths bypass dialect normalization. Initial-state-only coverage cannot prove the update paths.
- A successful JSON prefix does not establish complete output after overflow.
- Cache scope and generation fences must survive both successful and failed listings.

## Parallelism

`sequential`

## Inputs

- [Native catalog acquisition design](../../specs/agents/system-design/grouped-model-catalog.md#native-catalog-acquisition)
- [Projection and cache ownership](../../specs/agents/system-design/grouped-model-catalog.md#projection-and-cache-ownership)
- Task 02's qualification record and observed fixture
- Existing process ownership, ACP cleanup, profile cache, config conversion, and restoration tests

## Results

Blocked on Task 02's captured-fixture qualification.
No production acquisition source, adapter plumbing, or process manager change was made, because the qualified native shape and launch context cannot be established from this workspace.
The pure enrichment boundary that this work order would call already exists in `apps/backend/internal/agentctl/acpcompat/devin_catalog.go`.
Resume only after a sanitized captured fixture qualifies Task 02.
