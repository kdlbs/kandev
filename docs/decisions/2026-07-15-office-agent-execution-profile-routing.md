# ADR-2026-07-15-office-agent-execution-profile-routing: Separate Office identity from routed execution profiles

**Status:** proposed  
**Date:** 2026-07-15  
**Area:** backend, frontend

## Context

ADR 0005 unified shallow CLI profiles and rich Office agents into one `agent_profiles` table. That removed duplicate identity rows and made Office agents usable by the shared task and workflow model. It also left one row carrying two responsibilities:

1. Durable Office identity: name, role, hierarchy, instructions, skills, Office permissions, budget, status, and history.
2. Concrete execution configuration: CLI/provider registration, account environment, model, mode, ACP config options, CLI flags and permission behavior, passthrough mode, and MCP configuration.

Provider routing currently changes a subset of responsibility (2) with model/mode/flags/environment overlays while launching the rich Office row. That is not sufficient for cross-provider fallback. A Claude profile and a Codex profile may use different credentials, CLI flags, permission behavior, config options, passthrough mode, and MCP configuration. Applying provider/model overlays to a Codex-authored base row can launch Claude with an invalid mixture.

The same ambiguity also makes provider changes modify or copy fields on the Office row even though the user's intent is to keep a stable role and only change how that role executes.

## Decision

Keep the unified physical `agent_profiles` table from ADR 0005, but recognize two logical roles and carry both IDs through Office launches:

- `agent_profile_id` is the stable Office identity for Office tasks. It owns role metadata, hierarchy, instructions, skills, Office permissions, budget, status, executor preference, task/run ownership, costs, and Office capabilities.
- `execution_profile_id` is the concrete profile selected for one launch. It owns the CLI/provider, credentials and environment, model, mode, ACP config options, CLI flags and CLI permission behavior, passthrough mode, and `agent_profile_mcp_configs` entry.

Non-Office launches remain compatible by treating a missing distinct execution ID as `execution_profile_id = agent_profile_id`.

### Routing references profiles

Workspace provider-tier mappings store execution profile IDs as authoritative references. Provider and model are derived snapshots for validation, preview, health scope, and audit. Routing does not construct a launch by combining a base Office profile with provider-level mode/flags/environment overlays.

The canonical JSON field is `execution_profile_ids`. The decoder accepts legacy `tier_profile_ids` during migration. Legacy model-only mappings are converted only when an active profile match is unique; otherwise settings show a missing/ambiguous mapping that requires user selection.

An execution profile reference must:

- exist and not be deleted;
- be a shallow runtime profile rather than a rich Office identity;
- be global or belong to the same workspace;
- carry a launchable CLI configuration; and
- resolve to the logical provider that owns the mapping.

Profile deletion remains blocked while a routing mapping references it.

### Tier selection is always active

The workspace `enabled` flag controls automatic fallback, not whether Office uses tier profiles.

- When `enabled=false`, resolution computes the effective tier and launches only the first provider's mapped execution profile. It does not health-filter or try another provider.
- When `enabled=true`, resolution walks the ordered healthy candidates and uses the existing degradation, retry, and parking policy.

An invalid first mapping is an actionable configuration error. Office does not silently launch the workspace default profile.

### Runtime and persistence boundary

Task assignment and Office ownership continue to use the stable `agent_profile_id`. The concrete `execution_profile_id` is persisted on route attempts, runs, task sessions, and `executors_running`.

Lifecycle resolution uses `execution_profile_id` for process configuration and `agent_profile_id` for Office materialization and authorization. Kandev-owned identity environment such as `KANDEV_AGENT_PROFILE_ID` cannot be overridden by an execution profile; an optional `KANDEV_EXECUTION_PROFILE_ID` exposes the selected runtime profile for diagnostics.

Persisting the execution profile on `executors_running` binds provider-native resume state to the profile that created it. If fallback selects another profile, Kandev clears or ignores the prior ACP/session token, refreshes the execution snapshot, and starts a fresh provider-native session.

### Cross-provider continuation

Post-start fallback reuses the same task, run, task environment, and worktree. It reapplies the stable Office agent's instructions and skills and adds a continuation instruction telling the replacement agent to inspect task description, comments/messages, status, prior run state, and current git state before continuing.

Provider-native chat state is not portable and is not transferred. Durable task state and repository state are the handoff contract.

## Consequences

### Positive

- A custom Office role keeps its instructions, skills, permissions, budget, and history while switching providers.
- Codex-to-Claude fallback uses the complete Claude profile, including its credentials, flags, config options, permission behavior, and MCP servers.
- Route history and restart recovery identify the exact execution profile used.
- Existing task/workflow references remain stable because ADR 0005's unified table and Office identity IDs are preserved.

### Costs

- Launch APIs and persistence must carry two IDs explicitly.
- Routing validation now depends on loading referenced execution profiles.
- Session recovery must reject resume tokens when the execution profile changes.
- The routing UI must select profiles rather than accept model-only mappings.
- Existing copy-on-select behavior for Office agent configuration is superseded and must be removed or migrated without reverting unrelated fixes.

## Alternatives considered

### Reintroduce `office_agent_instances`

Rejected. It creates a clean table boundary but reverses ADR 0005, duplicates identity across task/workflow systems, and requires broad foreign-key and API migration. Two explicit IDs provide the required runtime boundary without restoring the old table split.

### Continue applying route overlays to the Office row

Rejected. Provider/model/mode/flags/environment overlays cannot represent complete CLI profiles and can leak provider-specific credentials, config, permissions, passthrough behavior, or MCP configuration across a fallback.

### Copy the selected execution profile into every Office agent

Rejected. Copies drift, make provider switching mutate identity rows, and cannot represent an ordered fallback chain without repeatedly rewriting the Office agent.

## Relationship to prior decisions

This decision amends ADR 0005. The physical table unification and stable Office identity remain. The statement that one row is the complete launch source no longer applies to routed Office execution: a rich row is the logical identity, and a separately referenced profile is the concrete runtime source.
