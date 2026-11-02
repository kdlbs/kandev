---
status: draft
system: agents
created: 2026-10-30
requirements:
  - REQ-AGENTS-SANDBOX-011
  - REQ-AGENTS-SANDBOX-012
---

# Typed sandbox operations and provider upgrade acceptance

## Ownership and mapping

Agents owns permission meaning, operator authority, and attempt lifecycle. REQ-AGENTS-SANDBOX-011 maps to models, admission, operation paths, and review UI. REQ-AGENTS-SANDBOX-012 maps to migration and real acceptance. [Baseline policies](sandbox-policies.md), [evidence](sandbox-evidence.md), and [provider major 2](../../plugins/system-design/sandbox-provider-v2.md) retain their respective boundaries. The [decision](../../../decisions/2026-10-30-sandbox-evidence-and-operation-authority.md) records alternatives.

## Policy model

Introduce explicit schema version 2 in the common sandbox policy model. Separate `static_grants`, `approval_envelope`, and executor `ceiling`. The task guard constrains both static access and possible approved expansion. A selected policy's static grants and envelope must each be subsets of the ceiling and captured task limit. Inherited hard denials are never removed.

Operations are a tagged union, never an overloaded target string:

| Kind | Canonical identity and initial supported matching |
| --- | --- |
| Filesystem | Existing resolved root and read/write mode; preserve protected-root checks |
| TCP destination | Canonical DNS name or IP, exact port, TCP transport; no wildcard or application-protocol promise |
| Command | Operator-defined command ID, immutable executable identity, exact argv vector, declared caller edge, working-directory identity, and referenced immutable resource policy |
| HTTP endpoint | Route ID, exact HTTPS origin including port, uppercase method, exact normalized path, and explicit query policy |

Command resource policies contain their own filesystem, TCP, environment-key, and explicit child-command permissions. They must fit the executor ceiling and the task's approved expansion envelope. Nested command edges are explicit and cycle-checked. The initial UI supports exact argv and caller edges, not arbitrary regular expressions, shell snippets, or provider profile text. Shell/interpreter invocations require explicit resource confinement; inspecting a command string does not prove safety.

Endpoint rules initially use an exact path and no query, or an exact operator-reviewed query with explicit keys and values. Secret-bearing targets cannot be stored as policy literals. Reject ambiguous encodings, duplicate query keys, dot segments, userinfo, fragments, encoded separators, and unsupported normalization. Path prefixes/globs, arbitrary CONNECT, websocket upgrades, and body-level authorization are excluded. HTTP approval does not assert anything about request-body content. Surface that limit.

A route constrained at HTTP level cannot also receive a raw TCP grant or network approval envelope that bypasses its method/path controls. Reject collisions during canonicalization, compilation, and admission. Logical Kandev MCP remains its own capability, not an editable HTTP route to the backend.

Extend `internal/common/sandbox` canonicalization, subset comparison, resolved grants, task guards, policy digests, learning diffs, and server validation together. UI restrictions never substitute for service checks.

## Migration and persistence

Retain schema 1 decoding and its original hashes. A schema 1 filesystem approval uses its existing captured ceiling semantics. Never silently convert that ceiling into a schema 2 expansion envelope. New schema 2 policies default to no expansion.

An operator can explicitly migrate a revision. Review the full static-grant and approval-envelope diff, then publish an immutable schema 2 revision and explicitly select it. Existing attempts keep protocol, package, runtime, and policy identities. Unknown versions refuse new launch; they do not erase fields or retry through v1.

Add schema-tagged operation and caller binding to approval/report persistence using the existing SQLite/PostgreSQL stores and migrations. Legacy rows remain readable as filesystem operations. Do not rewrite finalized decisions or historical receipts. Core captures a digest of the complete normalized request for replay protection.

## Request identity and decisions

Keep callback secrets private and isolate callbacks from child filesystem, procfs, inherited descriptors, proxy destinations, and environment. Each operation class uses a declared attempt-owned channel. Core chooses the channel ID and rotates its secret with each attempt. The provider can bind the protected reference into configuration, but cannot receive its secret through RPC output or public state.

The normalized actor has two alternatives:

- `child_process`: provider session plus actual requesting PID. Core verifies birth identity, containment membership, and permitted caller lineage. A mediated command's requester may be a shim descendant; do not substitute the session-root PID.
- `runtime_service`: host channel ID plus declared service/route identity. The callback must arrive on that channel, and core verifies live owned supervisor/containment and the route compiled for this attempt. PID zero remains absent. A shared Nono label such as `proxy` is not an authentication factor.

Extend `approval_validation.go` and the runtime approval guard through an explicit actor switch. Never weaken the old filesystem path to accept PID zero. Persist only the non-secret binding. Reject foreign route IDs, unknown channels, stale generations, changed request bodies, and unavailable owners before prompting.

For an operation already inside frozen static grants, core can authorize that exact request as `static_policy`, with an audit record. This is evaluation of existing operator policy, not agent auto-approval. Otherwise require membership in the captured approval envelope and current ceiling before persisting a pending operator request. Read-only, synthetic, agent, and Office identities cannot decide it. Recheck authority, revocation, attempt liveness, and request revision immediately before sending the decision.

A decision applies once to its request ID. Keep the 60-second total host budget, duplicate-body checks, uncertain-delivery state, and denial on timeout/cancellation. No automatic retry after an uncertain grant. Record decision and delivery separately from any later runtime event. Never claim operation completion solely because a reply was sent.

## Network path

Nono 0.81.0's direct CONNECT path supports network approval backends. Its plain `allow_domain` still strips ports, and forwarded upstream CONNECT requests do not consult the new backend. Do not infer exact-port safety from the release title.

For capability-selected v2 network mode, compile an empty static domain allowlist with a nonempty private webhook backend. Each eligible CONNECT asks core, which evaluates exact host/port against static policy or the approval envelope. Do not configure the old upstream relay in this mode. Nono must re-resolve and recheck the destination before connecting. The provider declares `network_preflight` and one-connection semantics only after conformance passes.

No `--allow-net`, legacy empty-list allow-all, or inherited hard-block override is permitted. If DNS, link-local, metadata, protected callback, private service, or policy checks deny a destination, approval cannot override that denial. Logical MCP uses only the current host-supplied endpoint and its scoped relay. Required endpoint incompatibility blocks launch rather than admitting all loopback traffic.

The old exact TCP egress relay remains the static v1 implementation. Selecting v2 is explicit. A failed v2 capability check must not switch modes. Tests must include same host/different port, hostname aliases, direct IP, IPv6, DNS changes, proxy bypass, callback access, concurrent requests, and a second connection after one grant.

One approved connection may carry multiple HTTP requests or a long-lived protocol. The UI states this. A connection approval is not a one-request HTTP grant.

## Command path

Nono's broker owns invocation interception and inner tool sandboxes; core retains outer supervisor/PTY and containment ownership. The provider compiles command identities and caller edges into Nono command policies, with exact executable paths and no writable-executable or direct-exec bypass escape hatches.

Require evidence that the requested executable, caller edge, argv, working directory, and resource policy match the compiled invocation. Mutable binaries, symlink replacement, scripts/interpreters, direct canonical-path bypass, and unmediated descendants are negative tests. If a supported binary identity cannot be held stable through execution, refuse the affected command policy.

An approved invocation does not grant the agent shell unrestricted execution. Tool resource grants cannot inherit the parent's broad grants implicitly. Report the controlled command scope, not a claim that every executable in the session is mediated. Repeated invocations require fresh decisions. Unsupported chaining stays denied, and cancellation terminates brokered descendants in the owned containment.

## HTTP endpoint path

Use Nono's configured route/endpoint mediation, with default deny and exact origin/method/path rules. The provider must map each callback's route and upstream to the compiled route descriptor. Ignore arbitrary callback URLs as authority.

For HTTPS interception, create a private per-attempt CA and child trust bundle through declared runtime artifacts. CA private keys stay in protected supervisor state. Never install a global trust root or disable certificate verification. Tools with incompatible certificate pinning fail explicitly. Cross-origin redirects and every new endpoint request are re-evaluated. Raw TCP fallback to a restricted origin is denied.

Nono endpoint callbacks use `session_id: proxy` and PID zero in inspected 0.81.0 source. Use the runtime-service channel binding above. Prove callback isolation and route substitution refusal with the actual runtime before advertising endpoint support. Missing evidence leaves the capability unavailable, not partially enabled.

## Settings, decisions, and learning UI

Extend existing executor policy editors, `SandboxApprovalReview`, report views, and learning review with typed fields. Show static grants and Ask for approval separately. A TCP row displays host and port. A command row displays immutable identity, exact arguments, caller, and resource-policy summary. An endpoint row displays origin, method, and path/query scope.

The review header names the operation type and attempt. Deny and Allow once stay explicit actions. Network copy says one connection; command copy says one invocation; endpoint copy says one request. Dangerous raw argument values remain private; when redaction prevents a meaningful exact-scope review, refuse approval and explain the missing review information.

Reuse the evidence design's desktop dialog and phone full-height next-view composition. One internal scroller, fixed Back/title and decision actions, safe-area clearance, at least 44px phone hit targets, and 28px desktop controls. Long targets wrap. No hover-only details. Preserve the same pending request and unsaved policy draft across breakpoint changes. Localize all labels and failure explanations.

Learning proposes typed additions only from exact supported records. Adding an eligible operation to the approval envelope and adding a static grant are different changes. Default the proposal to the narrowest supported choice and require the operator to select its persistence meaning. Publishing a revision does not alter current attempts or automatically restart.

## Upgrade and real acceptance

First migrate the dedicated plugin to the public standalone SDK without changing behavior. Then select and verify the 0.81.0 source/archive/binary checksums and review relevant release changes. Update runtime-lock, package checks, and CI pin together only after baseline conformance. Keep 0.80.0 receipts immutable.

Run tests in a disposable owned instance, never main :9998 or retained instances. Use explicitly supplied fixture credentials privately. For each enabled class prove allowed work, denial, timeout, malformed reply, replay, expired owner, concurrency, cancellation, and sibling isolation. Pair each grant with a nearby forbidden operation. Re-run discovery, ACP, native input/resize, dynamic MCP, same-session restart, and cleanup with exact package/runtime digests.

Phase completion requires the planned real tests, not merely a capability flag or mock browser result. If an upstream behavior cannot meet the reviewed contract, retain the refusal and report the specific blocker before changing the design. The [plan](../../../plans/nono-sandbox-expansion/plan.md) owns delivery status.
