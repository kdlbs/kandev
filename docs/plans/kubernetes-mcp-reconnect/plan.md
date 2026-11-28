---
created: 2026-11-27
status: done
requirements:
  - REQ-AGENTS-BUILTIN-MCP-003
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
legacy_specs: []
---

# Kubernetes built-in MCP reconnect

## Outcome and scope

Restore the existing execution-binding contract across Kubernetes reconnect,
helper recreation, and repeated active recovery. A saved desired profile must
not relabel a retained helper or rewind a later admitted binding.

The [requirements](../../specs/agents/requirements/builtin-mcp-management.md)
and [design](../../specs/agents/system-design/builtin-mcp-management.md#execution-binding-and-compatibility)
own the authority model. This fix changes no package grants, authentication
model, protocol, persistence schema, or execution boundary.

## Implementation

[Task 01: Preserve binding ownership](task-01-preserve-binding.md) is the only
work order. Reuse existing Kubernetes lifecycle and verification mechanisms;
no new controller, configuration, dependency, or migration is needed.

Copy request metadata before restoring a retained binding. Preserve the
matching creation profile, keep session authority out of shared environment
inventory, and derive active refresh authority from the current execution.
Existing ownership and helper confirmation checks remain fail closed.

## Verification and risks

Use failing regressions for retained profile drift, environment contamination,
foreign owners, malformed bindings, and repeated recovery. Run affected Go
race checks and repository lint. Verify actual helper bindings and MCP calls
in a disposable Kubernetes cluster, including retained transport, container
restart, and task idle cleanup with preserved storage identity.

The main risk is confusing normal launch policy with active execution recovery.
The work order records proof for both original and subsequently admitted
bindings. Local validation does not establish provider conversation continuation
or a deployed session's precise failure cause. Hosted CI and upstream integration
remain separate from local completion.
