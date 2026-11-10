---
status: current
system: cli
requirements:
  - REQ-CLI-VERSION-JSON-001
---

# Native CLI version JSON system design

## Purpose and boundaries

The native launcher owns the public invocation, consistent with the
[Go launcher decision](../../../decisions/2026-08-08-go-launcher-owns-all-launch-modes.md).
The npm shim forwards argv to that binary. Build stamping and backend metadata
remain independent contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-CLI-VERSION-JSON-001` | Components, data and contracts, control flow, failure and recovery |

## Components, data and contracts

`cmd/kandev/main.go` supplies `launcher.BuildInfo` from `Version`, `Commit`,
and `BuildTime`; its unstamped defaults are `dev`, `unknown`, and `unknown`.
Existing release ldflags provide stamps. `internal/launcher/options.go` records
the JSON flag in ordinary launcher options and validates its dependency on the
version flag after parsing all arguments. Existing argument validation remains
in force. `internal/launcher/cli/help.go` and `docs/public/cli.md` describe the
opt-in format near the version option.

The launcher normalizes version with the existing `normalizedBuildVersion`;
it replaces only empty commit and build-time stamps with `unknown`, without
trimming or altering nonempty values. A dedicated JSON object with explicit
`version`, `commit`, and `build_time` tags is encoded with `encoding/json` and
one terminating newline. JSON escaping preserves decoded stamp values. The
field set is exactly those three strings; plain output keeps its existing path.

## Control flow

`launcher.Run` continues to dispatch `service` and `maintenance` first, so
maintenance's own `--json` parsing is unaffected. Ordinary parsing accepts the
two flags in either order, rejects standalone JSON, and retains other parser
errors. After parsing and normalization, the version branch returns before
help or any runtime startup, config, network, data or child-process work. The
plain version branch retains its existing print and signal behavior.

## Failure and recovery

For JSON version output alone, invoke the existing `ignoreBrokenPipeSignal`
before writing, then return 1 and emit a stderr diagnostic on write failure.
Use a narrowly scoped writer or equivalent to detect an incomplete write even
when a writer returns no error, as the maintenance JSON writer does. Do not
change global launcher signal or stdout handling. The Unix broken-pipe test
uses an isolated subprocess and a SIGPIPE control, following the maintenance
test; writer tests cover ordinary errors and short writes. Usage errors retain
the existing parse error path and exit 2.

## Persistence, security and observability

The output is derived only from already supplied build metadata; it persists
nothing, needs no new privilege or telemetry, and exposes no configuration.
No JSON object is promised after an output failure.

## Related contracts

- [Native Kandev CLI requirements](../requirements/native-kandev-cli.md)
- [Version JSON requirements](../requirements/version-json.md)
