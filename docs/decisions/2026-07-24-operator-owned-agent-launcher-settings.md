# ADR-2026-07-24-operator-owned-agent-launcher-settings: Operator-Owned Agent Launcher Settings

**Status:** accepted
**Date:** 2026-07-24
**Area:** backend, frontend, protocol

## Context

Agent execution profiles contain inputs that affect the process before an ACP
agent is confined, including the launcher command prefix and the environment
inherited by that launcher. Office agents receive a runtime JWT and the backend
API URL, but the general agent-profile mutation routes currently require no
operator authentication; omitting the JWT is treated as an administrative UI
request. Kandev also has no application-level operator session or secure
browser bootstrap: the current web UI, HTTP API, and WebSocket are intentionally
unauthenticated, while the desktop health token and agentctl tokens authenticate
different process boundaries.

A per-boot value exposed through the SPA shell or `/api/v1/app-state` can
mitigate browser CSRF, but it is not operator authentication. Any agent or
direct client that can reach the backend can read and replay that value. This
limitation is also documented by the unfinished plugin boot-token work in
[PR #1808](https://github.com/kdlbs/kandev/pull/1808).

## Decision

Launcher-affecting definitions and agent-profile settings are operator
control-plane data. Agent processes and Office runtime JWTs are data-plane
principals and MUST NOT authorize their mutation. The protected resources and
required authority are:

| Resource | Examples | Required authority |
| --- | --- | --- |
| Install-wide agent definitions | custom TUI commands in `agents.tui_config`, agent creation/deletion, registry-backed executable selection | install operator |
| Global execution profiles | `agent_profiles` rows with an empty `workspace_id`, including nested MCP configuration | install operator |
| Workspace execution profiles | `agent_profiles` rows with a non-empty `workspace_id`, including nested MCP configuration | operator authorized for that workspace |

Workspace operators cannot mutate shared global definitions or profiles.
Creating a resource, deleting it, replacing it, or moving it between ownership
scopes requires the authority for both its old and new scope.

Within those resources, the boundary includes at least:

- launcher executable and command-prefix argv;
- CLI arguments and mode/configuration that select the launched program's
  behavior;
- profile environment values or secret references inherited by the launcher;
- profile creation, deletion, and any replacement operation that can select
  different launcher inputs.

These mutations require a mandatory authenticated operator principal. An
absent credential is unauthenticated, not an implicit administrator. An Office
agent JWT is rejected even when its role is `ceo` or otherwise administrative
inside the Office data plane. Authorization also validates that the operator
can mutate the target workspace/profile when Kandev supports more than one
operator or tenant.

Full-detail reads of the protected resources require the same operator
authority when they include literal environment values, resolved secret
material, MCP server environment, or MCP headers. Agent/data-plane callers may
receive a separate redacted catalog DTO containing identifiers, names,
compatibility/capability metadata, and secret-reference presence, but never the
literal values used by another execution profile.

The operator credential and its bootstrap material MUST NOT be placed in agent
process environments, workspaces, executor metadata, logs, Office runtime
responses, or an unauthenticated SPA/boot payload. A same-origin boot token may
still be used as a CSRF control, but it never satisfies this authentication
requirement. Deployments may satisfy the principal requirement with native
Kandev application authentication or with a trusted external identity proxy,
provided direct backend access is blocked and Kandev verifies a non-forgeable
proxy assertion.

Ambient browser credentials are safe only after agent-controlled and
workspace-controlled web content is isolated from the operator origin. Preview
ports, task applications, plugin content, and other untrusted scripts MUST use
a separate origin that cannot read operator boot/session state or send an
operator credential to the control-plane origin. Cookie path scoping,
iframe sandbox flags combined with `allow-same-origin`, and CSRF headers do not
replace this origin boundary.

Until that operator-authentication surface exists, command prefixes are a
launch customization feature, not a security boundary. A partial profile-route
guard based on optional Office JWT middleware, `Origin`/CORS, client IP,
user-agent headers, or a secret returned by an unauthenticated endpoint MUST
NOT be described or tested as operator authentication.

As an interim risk reduction, state-changing agent/settings routes may require
a random per-boot token exposed to the same-origin SPA and reject requests that
carry an Office bearer token. This interlock reduces accidental agent writes,
straightforward use of the injected runtime JWT, and cross-origin request
forgery. It does not stop an intentional agent or direct client from fetching
the unauthenticated boot payload and replaying the token, and therefore does not
satisfy the operator principal, secret-read, workspace-authorization, or
untrusted-origin requirements above. Tests and public documentation for the
interlock must preserve that distinction.

Execution-critical commands cross lifecycle and agentctl boundaries as
structured argv. The additive configure fields are `agent_args []string` and
`continue_args []string`; the existing `command` and `continue_command` fields
remain display/legacy compatibility strings. During migration, senders
dual-write both representations. Receivers prefer structured argv whenever its
field is present, reject present-but-empty or otherwise invalid argv without
falling back, and parse the legacy string only when the corresponding argv
field is absent. The same precedence applies to initial launch,
continue/one-shot execution, restart/context reset, and recovered executions.
Human-readable command strings are never reparsed when structured argv is
available. Legacy fields may be removed only after all supported lifecycle and
agentctl version combinations send and consume structured argv.

## Consequences

- Correctly hardening agent-profile mutation depends on a broader
  application-authentication and browser-session design. That work must cover
  native launcher, desktop WebView, ordinary browser, headless service, and
  mobile access without putting the credential in agent-visible surfaces. It
  must also move untrusted preview/content rendering off the operator origin.
- The post-merge launcher hardening cannot claim a security boundary until
  that dependency lands. Structured argv, restart ordering, and rollback tests
  remain valid follow-up work, but shipping them alone does not satisfy the
  launcher/settings acceptance criteria.
- The interim boot-token interlock is intentionally compatible with later
  operator authentication: it remains a CSRF layer or is removed once the
  authenticated session supplies a stronger principal.
- Existing settings clients will need an authenticated mutation transport and
  explicit handling for unauthenticated/forbidden responses. Read-only
  agent-facing discovery requires a dedicated redacted DTO; existing
  full-detail profile and MCP responses remain operator-only.
- Profile environment values may continue to be applied to the launcher only
  after operator-only mutation is enforced. If agents later gain a supported
  way to propose child environment values, those values require a separate
  post-confinement channel or an explicit sanitization contract.
- External-proxy deployments must prevent bypassing the proxy by connecting
  directly to the backend.

## Alternatives Considered

1. **Apply the current optional `AgentAuthMiddleware`.** Rejected because a
   caller can omit its bearer token and is then treated as an administrative
   UI request.
2. **Expose a random per-boot token in the SPA shell or boot payload.** Rejected
   as authentication because an agent can fetch the same unauthenticated
   resource and replay the token. Accepted only as the explicitly limited
   interim interlock and future CSRF protection described above.
3. **Protect only `command_prefix`.** Rejected because profile environment
   values can influence the wrapper through `LD_PRELOAD`, `PATH`,
   wrapper-specific variables, and interpreter resolution before confinement.
4. **Rely on CORS, `Origin`, loopback binding, source IP, or browser headers.**
   Rejected because non-browser agent clients can omit or forge those signals,
   and local executors may share the host network namespace.
5. **Move launcher policy to another unauthenticated HTTP route or table.**
   Rejected because changing storage ownership without authenticating the
   mutation principal preserves the same confused-deputy boundary.
6. **Use an ambient operator cookie while serving previews on the backend
   origin.** Rejected because agent-controlled same-origin scripts can exercise
   the credential and read CSRF/bootstrap material. Authentication and
   untrusted-content origin isolation must land together.
