---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-AGENTCTL-AUTH-MODES-001
---

# Agentctl authentication modes

## Purpose and boundaries

The agentctl control server and its instance servers use the authentication mode selected at process startup. The existing Sprite executor uses tokenless mode through the Sprite transport.

This design preserves that compatibility contract. It does not change bearer validation, credential rotation, or the authority of the Sprite transport.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-AGENTCTL-AUTH-MODES-001 | Startup wiring, Compatibility, Verification |

## Startup wiring

`cmd/agentctl/main.go:run` constructs the control server and registers the instance server factory. The factory currently attaches `ControlServer.CredentialSource()` to every instance.

The control server creates a credential source even when `Config.AuthToken` is empty. Its `controlCredentialAuth` middleware preserves tokenless startup in that case.

The instance middleware has a different condition. `Server.instanceAuth` permits tokenless requests only when both the static token and credential source are absent.

The correction belongs at the factory boundary in `run`. Attach the control server's credential source only when the process starts with a nonempty `Config.AuthToken`.

For tokenless startup, leave `Server.credentialSource` nil. The existing empty static token then selects the existing middleware behavior.

For authenticated startup, keep the existing shared source and atomic `AcceptsFullWithInvalidation` checks. Instances created before and after rotation must use that source.

Use the resolved startup configuration, including bootstrap-generated tokens. Do not inspect a request header, executor name, feature toggle, or authentication failure to select the mode.

Do not add an empty-token exception inside authenticated request validation. Do not replace the rotating source with a captured static token.

## Compatibility

| Consumer or shape | Transport and identity | Required behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Existing Sprite launch | Authenticated Sprite transport forwards to a tokenless agentctl instance | Configure, status, and agent stream accept tokenless requests | Real agentctl test with tokenless startup. No token migration |
| Local and worktree launch | Local HTTP/WebSocket with bootstrap-generated bearer token | Protected operations require the current credential | Real startup test plus existing rotation tests. Invalid credentials return 401 |
| Docker, SSH, Kubernetes launch | Executor-specific transport with configured bearer token | Shared instance wiring preserves authenticated mode | Shared startup tests cover agentctl behavior. Executor provisioning remains outside this test |
| Plugin remote executor | Compatible agentctl and plugin transport | Resolved controller authentication mode governs access | Shared coverage is conditional on the same agentctl contract. No new plugin capability claim |
| Direct API test server | Nil source with static empty or nonempty token | Existing static-token behavior remains intact | Existing bearer middleware tests |

An unavailable or malformed credential in authenticated mode never triggers a tokenless retry.

## Security and lifecycle

`Config.ListenHost()` retains its current listener policy, including the loopback default for tokenless startup. Explicit listener overrides retain their current semantics.

The repair does not broaden any HTTP, WebSocket, MCP, or proxy exemption. It does not change transport credentials or provider credentials.

The shared credential source remains authoritative for authenticated instances. Rotation still rejects superseded credentials for normal instance requests and invalidates existing streams.

The mode is selected at process startup. Runtime conversion from tokenless mode to authenticated mode is outside this contract.

There are no schema, secret-store, registry, profile, or protocol changes. Existing Sprite workspace preservation and recovery actions remain unchanged.

## Verification

Exercise the production `run` path in an isolated subprocess. Do not recreate its server factory inside a test-only router.

The test creates an instance through the control API, then uses the real agentctl client for configuration, status, streaming, and a mock-agent turn. It covers tokenless and authenticated startup separately.

Use the existing mock-agent executable and a deterministic message scenario. Keep all files, ports, processes, and configuration inside the test fixture.

Authenticated cases must exercise both an existing instance and one created after rotation. Existing stream tests retain their race-sensitive invalidation coverage.

Cloud provisioning and provider authorization are separate from this deterministic transport test. A live Sprite smoke test can add evidence without replacing it.

## Observability and recovery

Retain existing status codes and bounded diagnostics. No token or raw environment value belongs in logs or assertions.

Bootstrap errors still use their current classification. This repair removes the inconsistent authentication rejection without redefining recovery messages.

Existing remote controllers keep their uploaded binary until their normal replacement lifecycle. Updating the backend alone does not prove that a retained controller contains the correction.

## Related contracts and delivery

- [Standalone single-driver requirements](../requirements/standalone-control-server-single-driver.md)
- [Standalone survival design](agent-survival-across-restart-01.md#single-driver)
- [Sprite startup fix package](../../../plans/sprite-agentctl-authentication/plan.md)

No new ADR is required. This correction restores an existing startup mode and preserves the authenticated ownership boundary.
