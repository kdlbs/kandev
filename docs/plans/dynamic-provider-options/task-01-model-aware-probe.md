---
id: dynamic-provider-options-01
title: Add model-aware capability probe
status: done
wave: 1
depends_on: []
plan: docs/plans/dynamic-provider-options/plan.md
spec: docs/specs/agents/requirements/dynamic-provider-options.md
---

# Add model-aware capability probe

## Scope

Extend the sessionless ACP utility probe so it can apply a requested model and
return the provider's complete normalized configuration-option snapshot. Keep
the existing no-context probe behavior unchanged.

## Acceptance conditions

1. A model-aware probe applies the requested model through the generic ACP
   session-model path and returns the full post-model config-option snapshot.
2. A provider that reports the snapshot through a config-update notification is
   handled within a bounded wait; unsupported or incomplete responses are
   reported without being presented as authoritative.
3. Utility tests cover response snapshots, notification fallback, model
   application failure, and ACP compatibility normalization.

## Files

- `apps/backend/internal/agentctl/server/utility/types.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/model_apply_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_probe_capabilities_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor_test.go`
- `apps/backend/internal/agentctl/acpcompat/` (only the shared normalization
  helper files that need extraction or reuse)

## Dependencies and inputs

- Existing ACP probe/session initialization and `sessionmodel` APIs.
- Existing `ClientCapabilityMeta` and ACP dialect normalization.
- The dynamic-provider-options spec and ADR.

## Output contract

Expose a utility-level model-resolution request/result that the host utility
manager can call without knowing provider-specific option identifiers. The
result distinguishes a complete empty snapshot from an error or incomplete
provider response.

## Checks

```bash
cd apps/backend && go test ./internal/agentctl/server/utility ./internal/agentctl/acpcompat
```

Results: `cd apps/backend && go test ./internal/agentctl/server/utility -count=1` — passed.
