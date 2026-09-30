---
id: "01-profile-probe-contract"
title: "Profile discovery API and probe isolation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.1
  - AC-AGENTS-PROFILE-DISCOVERY-001.2
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.4
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-001.6
  - AC-AGENTS-PROFILE-DISCOVERY-002.1
  - AC-AGENTS-PROFILE-DISCOVERY-002.2
  - AC-AGENTS-PROFILE-DISCOVERY-002.3
  - AC-AGENTS-PROFILE-DISCOVERY-002.4
  - AC-AGENTS-PROFILE-DISCOVERY-002.5
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
---
# Task 01: Profile discovery API and probe isolation

## Summary

Deliver authorized saved/draft profile discovery through the actual provider subprocess.
Baseline and model-option requests use one resolved launch context with separate bounded caches.
The context-free APIs remain compatible.

## In scope

- Add the typed profile probe POST route and optional launch context on the model resolver.
- Validate profile ownership, route identity, new-draft authority, env entries, secret references, flags, and prefix.
- Reuse environment and secret-resolution semantics without changing session precedence.
- Pass effective runtime command, env, CLI tokens, and prefix into the probe exactly once.
- Audit OpenCode helper enumeration, native utility probes, and command/diagnostic logs.
- Isolate cache and single-flight identity, bound retention, and fence refresh/runtime generations.
- Add regression tests first, including a real fake-provider subprocess that reacts to env and flags.

## Out of scope

- Editor integration, commercial model requests, package updates, new remote probes, and profile persistence changes.
- Changing gateway auth, workflow routing, or live session model selection.

## Acceptance

1. Saved and complete draft inputs reach both subprocess probe paths after existing authority and validation checks.
2. Distinct contexts, secret rotation, explicit refresh, and runtime activation cannot reuse or publish mismatched results.
3. Failures preserve existing settings and agent-wide snapshots, expose sanitized status, and clean up owned processes without prompts.

## Verification

Run from the repository root. Each command has an independent working directory.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/hostutility ./internal/agentctl/server/utility ./internal/agent/settings/cliflags)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/hostutility)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*(Env|Environment|Command|Profile|Inference)' -count=1)
git diff --check
```

Tests assert actual child environment and arguments, not only request serialization.
Controller and handler coverage uses `profile_discovery_test.go` and `model_config_handlers_test.go`.
Hostutility coverage uses `profile_capability_cache_test.go` and `profile_probe_test.go`.
The utility fake-child regression in `profile_probe_context_test.go` verifies environment, flags, and prefix delivery.
The tests also cover unavailable secret references, explicit clears, profile ownership, invalid request shapes,
cache bounds and expiry, transient failures, and unsupported native Codex context.

## Files likely touched

- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- New `apps/backend/internal/agent/settings/controller/profile_discovery.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/hostutility/{public.go,manager.go,types.go,model_config_cache.go}`
- New profile cache and context files under `apps/backend/internal/agent/hostutility/`
- Runtime-activation invalidation in the hostutility generation boundary
- `apps/backend/internal/agentctl/server/utility/{types.go,acp_executor.go,codex_app_server.go}`
- Shared environment resolution primitives identified from `lifecycle/environment_resolution.go` and `lifecycle/profile_env.go`
- Adjacent tests for all changed packages

## Dependencies

None. Read the paired design's API, authority, launch-context, cache, and failure sections first.

## Risks

- Existing utility prompt code omits unresolved secret references. It is not a complete template for profile discovery.
- Secondary OpenCode commands and native probes need context parity without weakening executable allowlists.
- A shared host instance must retain bounded operation admission while profile result caches stay separate.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/profile-capability-discovery.md), requirements 001 and 002.
- [Design](../../specs/agents/system-design/profile-capability-discovery.md).
- Existing `hostutility/manager_test.go`, `model_config_test.go`, and utility process/allowlist tests.
- Existing `hostutility.ExecuteProfilePrompt` shows DTO forwarding; lifecycle owns full secret and environment semantics.

## Results

Done. The profile probe route and model-option resolver share server-validated saved or draft launch settings.
Both probe paths receive the same environment, ordered CLI tokens, and validated command prefix.
Profile results use bounded context-specific caching and refresh generations; agent-wide snapshots remain separate.
Secret authority and incomplete launch snapshots fail before provider startup.

Verification passed:

- `go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/hostutility ./internal/agentctl/server/utility ./internal/agent/settings/cliflags -count=1`
- `go test -tags fts5 -race ./internal/agent/hostutility`
- `go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*(Env|Environment|Command|Profile|Inference)' -count=1`
- `git diff --check`

## Review remediation

The follow-up review found that explicit refreshes shared one agent-wide profile generation and that probes could return results after managed-runtime activation. The implementation now tracks explicit refresh generations per opaque complete profile context, while runtime activation retains its agent-wide generation. Cache reads and writes are serialized with generation checks and invalidation. The runtime generation is captured around managed-command resolution, and an obsolete command snapshot or probe result is rejected before use or return.

Deterministic regressions cover refreshing profile B without changing profile A's revision, runtime activation while a provider probe is blocked, and runtime activation while managed-command resolution is blocked. All three failed against the reviewed implementation before the fix and pass after it.

Additional verification passed:

- `make -C apps/backend build`
- `go test -tags fts5 -race ./internal/agent/hostutility -count=1`
- `go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/hostutility ./internal/agentctl/server/utility ./internal/agent/settings/cliflags -count=1`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The PR review follow-up also aligned probe environment construction with profile session launch: non-empty profile values and secret bindings override managed defaults, while empty unbound entries are ignored. Profile-context generation tracking is LRU-bounded to 256 revisions; eviction makes the old revision stale. The secondary OpenCode catalog command now uses bounded output-pipe waiting and process-tree cleanup. Regression tests cover both environment cases, generation eviction, and a cancelled provider descendant that retains output pipes.

Additional review verification passed:

- `go test -tags fts5 -race ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agentctl/server/utility -count=1`
- `make -C apps/backend build`
- `git diff --check`
