# Built-in MCP package delivery

`POST /api/v1/agent/configure` accepts an optional `project_mcp_delivery`
record (`agentctl/types.ProjectMCPDelivery`). It is the only way a project-file
strategy (Pi today) can satisfy package admission for a package the agent does
not accept over ACP. The record is authenticated, bound to the helper instance,
task, canonical workspace, and canonical package-binding digest, and validated
by `process.validateProjectMCPDelivery` before configured state changes;
omitting it clears prior evidence. Agentctl advertises
`project-mcp-delivery-v1`, and a backend must negotiate it before a dependent
launch. Pending evidence is consumed by one `buildAdapterConfig` call; the
adapter keeps the immutable clone for same-process load/resume/reset.
`acp.requireBuiltinMCPDelivery` accepts either an ACP-selected package or a
record entry for each requested package, never Core in place of a missing
optional package. `project_config_materialized` evidence reports the project
channel only; it never implies connection health.

Remote project files use the authenticated `/api/v1/mcp/project` contract:
`GET` returns the helper's own `WorkspacePath` and loopback `MCPURL`, `POST
/prepare` materializes generated files under `os.Root` containment with the
shared merge/ownership registry and read-back fingerprints, and `POST
/cleanup` releases that execution's claims. The backend must not apply a
container/Pod/SSH path it did not receive from the helper. Local and worktree
targets keep the existing backend writer.

## Native MCP route mode

The accepted create response records whether this helper serves scoped native
routes. A retained helper's mode comes from authenticated `InstanceInfo`. An
older helper that omits the field remains on bare routes; a failed mode read
must stop dependent delivery rather than trigger a bare-route retry.

In scoped mode, Core endpoints are `/i/<helper-id>/mcp`,
`/i/<helper-id>/sse`, and `/i/<helper-id>/message`. Optional package endpoints
append `/packages/<package-id>` to the corresponding transport path. The
instance prefix applies to HTTP and SSE transports, including package message
paths. The external backend MCP endpoint is unchanged.

The backend keeps the selected helper client with each delivery operation and
builds provider files, arguments, Cursor entries, and project-file targets from
that client's accepted ID and mode. Staged replacement delivery uses the staged
client, even while the execution still publishes its previous client. Helper
ID routing does not change task/session authorization or package admission.
