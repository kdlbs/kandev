# Agent MCP preparation compatibility evidence

## Evidence boundaries

This work uses three separate kinds of evidence:

- Backend lifecycle, profile and recovery tests use controlled command runners,
  stores, and a local fixture. They verify policy, ownership, safe progress
  records, and typed recovery behavior without proving compatibility with the
  installed Cursor binary.
- Desktop and mobile Playwright tests mock API routes. They verify discovery
  selection, preparation hydration, terminal lookup/navigation, and localized
  feedback. They do not exercise backend authentication, Cursor CLI behavior,
  or the native MCP connection.
- The opt-in native smoke below runs the installed Cursor binary against a local
  OAuth-capable MCP fixture. It verifies native CLI and ACP behavior while
  avoiding real provider operations.

Earlier native plugin identity, enabled-plugin inventory, and ownership evidence
is retained in [the Cursor plugin MCP import evidence](../cursor-plugin-mcp-import/compatibility-evidence.md).

## Native Cursor smoke

The smoke ran on 2026-09-28 using Cursor Agent `2026.09.26-dd393fe` on Darwin
arm64. It uses a disposable HOME and task workspace, a loopback HTTP MCP server,
and synthetic `fixture_ping` credentials. The actual Cursor binary performs
discovery and verification; its terminal TUI and ACP subprocess each invoke the
fixture tool. ACP approval is constrained to the one fixture tool and is
explicitly granted with `allow_once`.

The complete package passed:

```text
TestCursorNativeLifecyclePreparationAgainstAuthenticatedFixture (42.19s): PASS
TestCursorNativeLifecycleAuthenticationRecoveryLoadsSavedACPConversationInReplacementProcess (30.67s): PASS
Package lifecycle (73.772s): PASS
```

The recovery test first observes Cursor's real missing-authentication
`list-tools` diagnostic and the lifecycle's `authentication_required` result.
After writing only a synthetic fixture credential to the task auth file, it
repeats lifecycle preparation, stops the original ACP process, starts a
replacement process, and calls `session/load` with the prior saved session ID.
It does not call `session/new` on the replacement. The test checks that the
initial `READY` conversation remains available, ACP requests `allow_once` for
the exact fixture tool, and the replacement process makes a new successful
`tools/call` to the loopback server.

The fresh-auth test separately verifies lifecycle preparation and actual
`fixture_ping` calls through both the terminal CLI and ACP. It removes the
synthetic fixture's narrow CLI allow rule before ACP starts so the test observes
and grants the ACP permission request explicitly.

Run from `apps/backend` on macOS with the same installed binary and an existing
Cursor account token in the login keychain:

```bash
KANDEV_CURSOR_NATIVE_MCP_SMOKE=1 \
KANDEV_CURSOR_AGENT_BIN=/Users/cfl12/.local/share/cursor-agent/versions/2026.09.26-dd393fe/cursor-agent \
GOCACHE=/tmp/kandev-cursor-review-go-cache \
go test ./internal/agent/runtime/lifecycle \
  -run '^TestCursorNativeLifecycle' -count=1 -v -timeout 8m
```

The harness reads the account token directly from the macOS login keychain into
memory with a bounded command. It sets `AGENT_CLI_CREDENTIAL_STORE=memory`,
passes the account token only to the child process environment, scans the
temporary HOME before cleanup, and rejects a persisted account-token copy. The
fixture access token is synthetic. No Jira, SaaS MCP, personal MCP, or other
provider operation is performed.

## Supported claim

This evidence establishes that this Cursor Agent build can materialize and use
the selected native MCP server in its TUI and ACP modes, report the tested auth
failure, and reconnect a replacement ACP process by loading the same saved
conversation after synthetic credentials appear. It does not establish validity
of a production provider credential, provider authorization, or successful
business operations against a SaaS MCP server.
