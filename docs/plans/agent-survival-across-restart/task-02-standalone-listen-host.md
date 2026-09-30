---
id: "02-standalone-listen-host"
title: "Keep locally launched agentctl listeners on the standalone host"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-001
acceptance_criteria:
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.12
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
---

# Task 02: Keep locally launched agentctl listeners on the standalone host

## Summary

The standalone launcher starts agentctl with a bootstrap nonce, so agentctl
holds an auth token and binds its control server and every instance server to
all interfaces. The backend dials them only at `agent.standaloneHost`
(`127.0.0.1` by default). Pass that host to the child as
`AGENTCTL_LISTEN_HOST`, and make the launcher's fallback-port probe bind where
the child will listen instead of the wildcard address.

## In scope

- Add `AGENTCTL_LISTEN_HOST=<agent.standaloneHost>` to the launcher-owned
  child environment, replacing an inherited value.
- Bind the launcher's fallback control-port probe to the standalone host when
  it is a specific IP literal, and to `127.0.0.1:0` for `localhost`, other host
  names, and unspecified addresses.
- Update the `agent.standaloneHost` configuration row and the Windows
  firewall note in public docs.

## Out of scope

- The backend HTTP listener default (`server.host`, `0.0.0.0`).
- agentctl launched by the Docker, remote Docker, Sprites, SSH, and
  Kubernetes executors, which set their own environment.
- `Config.ListenHost` itself, the auth-disabled loopback fallback, and the
  websocket port tunnel bind.
- The shared task-process port allocator (`portutil.AllocatePort`).
- The `localhost` MCP URLs agentctl hands to agents.
- Listeners started inside agentctl for editors and previews.

## Acceptance

- `AC-EXECUTORS-CONTROL-OWNERSHIP-001.12` describes the behavior covered by
  this work order.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/agentctl/launcher -run "TestBuildAndStartProcess(PinsChildListenHost|ReplacesInheritedListenHost)|TestFreePortProbeAddrFollowsChildListenHost|TestFindFreePortOnLoopbackHosts|TestEnvironmentWithOverridesRemovesInheritedValues" -count=1
cd apps/backend && golangci-lint run ./internal/agent/runtime/agentctl/launcher/... ./internal/agentctl/server/config/... --new-from-rev=<base-sha> --timeout=10m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- The child environment carries exactly one `AGENTCTL_LISTEN_HOST` entry
  equal to the configured host for `127.0.0.1`, an empty host (`localhost`),
  `[::1]`, and a non-loopback address, including when the backend inherited a
  different value.
- Before the change the child environment had no entry, or kept the
  inherited `0.0.0.0`.
- The fallback-port probe address follows the host for IPv4 and IPv6
  literals and is `127.0.0.1:0` for `localhost`, an empty host, other host
  names, and unspecified addresses.
- The tests compose the child environment without starting a process, and
  bind only `127.0.0.1` and `[::1]`; the IPv6 case is skipped where IPv6
  loopback is unavailable.
