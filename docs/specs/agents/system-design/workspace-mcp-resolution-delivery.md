---
status: current
system: agents
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
---

# Workspace MCP setup and resolution delivery

This complements [workspace MCP configuration](workspace-mcp-configuration.md)
and its existing additive-selection and session-observability decisions. It
repairs missing handoffs in the original implementation; it does not add the
user's deferred management, inheritance-exclusion or neutral-status features.
Implementation: [review remediation](../../../plans/workspace-mcp-configuration/task-10-review-remediation.md).

## Store and migration boundaries

Use PostgreSQL REPEATABLE READ for a resolution spanning definitions, scope
selections and desired revision; SQLite retains its native read transaction.
The accepted snapshot and revision must come from that one read. Atomic
confirmed deletion rebinds every SQL parameter in the same transaction.

Legacy URLs containing userinfo, query or fragment components stay on the
pending/rebind path. A catalog projection strips those components; the source
legacy configuration remains unchanged for runtime fallback. Malformed URL
errors are generic, without copying the source URL. A stripped public value
must not imply that legacy authentication was migrated successfully.

## Setup representation and validation

Arguments remain a string array from editor hydration through save. One row is
one argument; no shell parsing, whitespace splitting or trimming of values.
Non-secret environment/header values and secret references are separate inputs.
Existing fields and bindings survive unrelated edits. Names are nonblank and
unique; incomplete new bindings prevent saving. A disabled definition selected
directly at a scope can be removed there; it cannot be newly selected. Inherited
rows retain the original additive policy.

Marketplace review shows requested values and workspace-visible secret metadata.
Required inputs block completion in both UI and service. The optional
`input_values` install field is limited to declared non-secret inputs; unknown
names, secret literals and ambiguous/duplicate bindings are rejected. Overrides
never mutate cached publisher metadata. Required secrets use references, and
reveal remains at runtime. Setup states that bound values reach the task
executor. Unmaterialized Registry variables, including nested input variables, and argument inputs without supported fixed values are unavailable with a localized reason. Decode retains their input metadata so eligibility and installation enforce the same restriction. Fixed named arguments preserve both the flag and its value; literal argument boundaries remain intact.

## Freeze each delivery operation

Construct a private prepared operation adjacent to resolution. It pairs the
exact ephemeral delivery server slice with cloned safe exclusions, setup and
desired revision. Raw resolved servers never enter stored execution metadata,
logs or browser events. Do not recover facts by rereading the catalog or an
execution's mutable latest resolution after another operation can replace it.
Cursor recovery retains the same prepared value in its validated recovery
target. Workspace rebind and rollback resolve their actual target contexts.

An authenticated ACP request carries optional safe resolution metadata separately
from executable MCP servers. Agentctl retains ownership of the new/load/resume/
reset attachment attempt. It derives task/session/execution identity from its
trusted configuration, bounds and sanitizes the metadata, and publishes excluded
rows under that exact attempt. Exclusions are never passed to the adapter as
executable placeholders. All four request families enforce the existing managed-conversation admission before provider delivery. Definitions removed by helper policy produce a safe filtered row. Requests omitting the field preserve existing behavior.
The host records the actual successful request's attachment ID, not a different
client's or concurrent request's mutable last-result slot. Capability admission
and RPC use pin the same runtime client. Release a bounded RPC client lease before entering a separate acceptance lease. Idle updates acquire startup callback ownership before the prompt lock and retain that order through acceptance; they never acquire a fresh startup read lease while holding the prompt lock. The detached idle update has a two-minute host deadline covering reconnect and runtime-setting restoration. Expiry releases its startup, prompt and client leases, preserves prior acceptance, and records safe failure under a separate five-second persistence budget. Unsupported reconnect defers the saved
selection before materialization; explicit supported-call failure remains failed. Providers whose underlying CLI consumes project MCP files also defer until the next process start, even when ACP advertises load/resume. A same-process acknowledgement does not prove that the CLI reread its project configuration.

CLI starts use the lifecycle operation owner shared by local and SSH runners.
Fresh start, actual resume start, failed-resume fallback and reset use one
prepared-operation/start handoff. Confirmed process identity and existing
execution/controller generation guards admit publication and acceptance. One
composed sanitized attempt describes delivered and excluded configuration; it
never claims connected or active tools from process start. SSH reuses the
existing idempotent start OperationID and lookup checks. Live-process adoption
creates no attachment and does not claim newly saved catalog state was delivered.
Shell terminals do not pass through this agent-specific producer.

Keep the configured/excluded evidence in the existing session-owned attachment
pipeline and API projection. Secret failure and executor filtering remain
visible even when healthy servers are accepted. Safe definition identity,
revision and origins travel with the operation; credentials, full URL, argv,
environment and headers do not. Delayed results cannot change a replacement
execution, process or attachment. Existing reducer bounds and sanitization apply.

The alternatives of executable placeholders, a second diagnostic RPC, and
reading a mutable latest-resolution field are rejected because they respectively
risk provider delivery, duplicate telemetry ownership, and cross-operation
attribution. This rationale fits the existing lifecycle/agentctl ownership
boundary and needs no separate ADR.

## Frontend request ownership

A save's completion/finally invalidates reads only while that same save and
scope context still own the editor. A late old-context completion cannot cancel
the replacement context's initial GET or clear its errors/loading state.

Phone forms use the existing one-scroller settings/review surface, stack complex
name/secret controls, wrap metadata and retain 44px touch actions. Fine-pointer
desktop controls retain established compact sizing. Shared Save changes and
cancel/discard ownership remain intact. All new copy is localized.
