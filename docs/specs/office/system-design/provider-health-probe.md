---
status: current
system: office
requirements:
  - REQ-OFFICE-PROVIDER-PROBE-001
---

# Office Provider Health Probe System Design

## Purpose and boundaries

`routingerr.ACPProbe` (`internal/agent/runtime/routingerr/acp_probe.go`) is the
`ProviderProber` registered for each routable provider. A command resolver supplied
at boot returns the argv and environment for a provider. The probe owns spawning,
the `initialize` round trip, and classification of the result.

The managed npm prefix contract belongs to `internal/common/managednpm`. Agent
launch paths already use it through `managedruntime.PrepareNPMProjectPrefix`. This
design makes the probe a consumer of the same contract. It adds no new prefix logic.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-PROVIDER-PROBE-001` | [Prefix resolution before spawn](#prefix-resolution-before-spawn) |

## Prefix resolution before spawn

`ACPProbe.Probe` copies the resolver's argv, then calls
`managednpm.PrepareNPMProjectPrefix` on the copy before building the command. That
function replaces the marker after `--prefix` with the private root under the system
temp directory and creates the root with mode 0700. It leaves argv without the
marker untouched. The copy keeps a shared or cached resolver slice from being
mutated.

## Failure behavior

If preparation fails, `Probe` returns `Classify` with `PhaseAuthCheck` and an
`acp-probe:` stderr, and spawns nothing.

## Tests

`acp_probe_prefix_test.go` covers the criteria with a fake binary that records its
argv, and `TMPDIR` pointed at a test directory.
