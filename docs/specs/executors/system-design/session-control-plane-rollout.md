---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
---

# Session control-plane rollout design

Companion to [session-control-plane-isolation](session-control-plane-isolation.md), whose
mechanisms this document gates. Section names below are those of that document.

## Rollout

The capability ships behind `features.sessionMcpToken` (`KANDEV_FEATURES_SESSION_MCP_TOKEN`),
`RestartRequired`, off in every profile (`prod`, `dev` and `e2e`), following the repository's runtime
feature flag procedure. The reason is that an unflagged change to every launch, recovery pass and
agent MCP configuration reaches every installation at once; the flag lets a maintainer enable it
on selected installations first.

Gating site. The backend reads the flag once at composition and passes a boolean to a single
session-MCP policy object; no other package reads the config field. The object answers: mint and
send the token (launch), verify (adoption), require the capability (control-server adoption),
rewrite the broker URL (relay), and whether the carrier rules and typed failures apply. With the
flag off each answer is "no" before any key, nonce, store or network access (AC-004.2), so
the create request, execution record and MCP configuration are the ones produced today. A parity test runs the
launch, recovery and control-server adoption paths with the flag off against recorded
expectations of the pre-capability behaviour (golden create-request body, metadata map, MCP
entries for every agent class including Cursor and Pi, and the broker URL in the agent
environment).

Policy object placement. `internal/common/sessionmcp` (created by this work; importable by the
backend and agentctl) holds the derivation, fingerprint and name constants. The policy object
(`SessionMCPPolicy`) lives in `internal/agent/runtime/lifecycle`, is built once in `backendapp`
composition from the flag, and is the only reader of the config field. The required-capability
set is computed at composition from the policy (today's `RequiredSurvivalCapabilities` plus `session_mcp_token` only when the flag is on), not at call time.
`RequiredSurvivalCapabilities` itself stays an unmodified package-level value (the runtime alias copies it at
package initialisation, so an append at composition would never reach it); `backendapp` passes the
policy-computed set into control-server adoption in place of the alias.

Parity scope. AC-004.2 compares what the flag-off backend writes for a launch whose record has
no `mcp_token_nonce` carried from earlier flag-on use. The carry-forward writers preserve an
existing nonce on a retained record whatever the flag (the leftover value authorizes nothing and is
harmless); they never create one.

agentctl is flag-free (AC-004.3). Its new behaviours are triggered by request content, each
with a no-token test showing today's behaviour:

| Behaviour | Trigger |
| --- | --- |
| Token guard `enforcing` | a token in the create request; otherwise `legacy` (unguarded, as today); a revoke moves either state, or an already revoked one, to `revoked` and wins over both |
| Broker relay, effective broker URL composition | an upstream URL in the create request; without one the `/git-credentials/` paths keep today's authorization outcome (`instanceAuth` applies, then 404 per AC-002.6) and the exemption from `instanceAuth` applies only to an instance that holds an upstream URL |
| Codex version probe, `KANDEV_MCP_SESSION_TOKEN` in the Codex process, `codex_wrapper` refusal | the instance is enforcing and the passthrough command line (argv elements joined with one space, so a wrapper shell's single string matches too) contains both `env_http_headers` and `KANDEV_MCP_SESSION_TOKEN` as substrings; the match runs first, so a wrapped Codex command reaches the `codex_wrapper` refusal instead of starting without the variable (built by the backend `CodexStrategy`, so a flag-off backend never emits it and none of these fire) |
| Revoke route, fingerprint-precondition header on delete | called only by a flag-on backend; an absent header is the unconditional delete of today |

Always-on changes, each tested for no effect without a token: the `/identity` additions (extra
fields and capability, additive), `Authorization` redaction in the ACP frame debug log, the
managed-policy acceptance of a single token header, and the HTTP 409 mapping for duplicate
instance IDs (a flag-off backend treats any non-2xx create as a launch failure, as it treated the
former 500, with a test).

Registry entry (copy is final; Build may not reword the risk): Label "Per-session MCP token and
Git broker relay"; Description "Requires a per-session token on each agent's Kandev MCP routes and
routes the Git credential broker through the session's own agentctl."; Stability
`StabilityExperimental`; RiskLevel `RiskHigh`; RiskDescription "Changes every launch, recovery
and agent MCP configuration. Retained sessions relaunch on the first restart with it on. Agents
whose MCP carrier is not verified (Cursor, Pi, custom TUI agents with a file strategy) and
Kubernetes and SSH retained instances fail to launch. Update operator-pinned agentctl images
first."; `RestartRequired` true; `Mutable` true; `Available` everywhere agentctl runs (no platform
exclusion). The frontend `features/types.ts` mirrors the key.

The flag is off in `dev` and `e2e` as in `prod`, following the repository rule that a release toggle
is off in every shipped profile. Tests that cover it enable it explicitly through
`KANDEV_FEATURES_SESSION_MCP_TOKEN=true` (Go tests set the config field directly), and an e2e spec enables
it only when it needs the capability. Consequences Build handles: the mock agents forward the configured
`Authorization` header (the ACP mock from the `session/new` `mcpServers` entry, the mock TUI agent from the
`headers` map of the Claude-class `--mcp-config` file, each pinned by a recorded-request test) so that a
spec which enables the flag can launch them; an enabling spec uses only agents whose carrier is available,
and a Kubernetes or SSH retained instance cannot be reused with the flag on (`retained_instance_deferred`,
AC-001.11). No existing spec changes its environment, and the executors documentation names these limits
for operators who enable the flag.

Compatibility matrix (backend flag, agentctl build, retained instance):

| Backend flag | agentctl | Result |
| --- | --- | --- |
| on | has `session_mcp_token` | Full behaviour. |
| on | lacks it (old, retained or operator-pinned) | Fresh create: `mcp_token_unsupported` after the AC-001.13 check. Retained local control server: replaced via `capability_incompatible`. Retained Sprites, remote Docker or plugin instance: mismatch, deleted and replaced (AC-001.11). Retained Kubernetes or SSH instance: not adopted, nothing deleted, revoke attempted (an old build has no route, so it answers 404 and the WARN notes the residual in the guard section), launch fails `mcp_token_unsupported` with `retained_instance_deferred`. |
| off | has it | Legacy instance; behaviour as today. |
| off | lacks it | Behaviour as today. |
| on to off, instances retained | has it | Retained enforcing instances are adopted without verification and keep their token. By carrier class when the agent restarts: ACP HTTP/SSE entries and Codex app-server headers are composed by agentctl, which still holds the token, so they keep working; a Claude or OpenCode passthrough file and the Codex passthrough `env_http_headers` argument are built by the flag-off backend without the token, so those Kandev MCP calls fail with the guard's `missing` refusal. A retained instance holding an upstream broker URL keeps relaying and composing the relay URL for agents it starts until replaced. Operators recover by stopping the session (instance replaced). Documented, tested per class. |
| on to off, instance already revoked (revoke route answered success) | has it | Adopted unverified (no verifier runs). Every MCP route for every carrier answers 503 `mcp_token_unconfigured`, including agentctl-composed entries, and the backend's stream keeps the instance active so the idle reaper does not collect it. Accepted and documented: operators stop the session (instance replaced) or stop sessions on Kubernetes and SSH before turning the flag off. Tested. |
| off to on | has it | Retained legacy instances have no fingerprint, so a local, Sprites, remote Docker or plugin one is classed as mismatch and replaced (AC-001.11); a Kubernetes or SSH one is not adopted: revoke (works on a current build, closing it to MCP), launch fails with `retained_instance_deferred` until it is reaped. |
| on to off to on | has it | For executors other than Kubernetes and SSH, instances retained from the first on period still verify against their persisted nonce and the persisted key and are adopted; instances launched during the off period are legacy and handled as in off to on. A Kubernetes or SSH instance is never adopted with the flag on (AC-001.11): it takes the off to on outcome whatever its history. Tested. |

The capability test pins `session_mcp_token` as advertised in `SurvivalCapabilities` under both
flag values, in the required set passed to control-server adoption only when the flag is on (the test asserts the set actually passed, not the package value), and each
matrix row.

End state. The intended end state is default-on with no runtime switch: a switch that
disables token enforcement is the bypass this capability removes, the launch-failing paths
are limited to the typed codes, and the only safe rollback is a revert. The flag is a
rollout device: after a release on which it is on for maintainers' installations without
incident, `prod` is set to on (the flag stays as an operator kill switch), and a later change
removes the flag and the legacy branches and appends the identity to the retired registry. The
maintainers decide when each step happens; this capability does not schedule them.
