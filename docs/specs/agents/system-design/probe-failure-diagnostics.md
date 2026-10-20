---
status: current
system: agents
created: 2026-10-14
updated: 2026-10-16
requirements:
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002
owners:
  - kandev
---
# Probe Failure Diagnostics System Design

## Purpose and boundaries

This design owns the technical contract for
REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001 and
REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002: how a one-shot probe subprocess's
stderr is captured, classified, sanitized, and written to the backend log, and
how the agentctl child's log format is pinned by the launcher that reads its
stdout.

It does not change client-visible probe error copy (owned by
[OpenAI-compatible providers](openai-compatible-providers.md)
for gateway status surfacing), probe command resolution (owned by
[Operator-Registered Agents](custom-acp-agents.md)),
or the launcher relay's record parsing, which is unchanged upstream behavior.

## Requirement mapping

| Acceptance criterion | Owner |
| --- | --- |
| AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.1 | `internal/agentctl/server/utility` probe failure logging |
| AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.2 | `internal/agentctl/server/utility` (`sanitizeProbeDiagnostic`) |
| AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.3 | `internal/agentctl/server/utility` (`sanitizeProfileProbeFailure`) |
| AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.4 | `internal/agentctl/server/utility` (`stderrBuffer`) |
| AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.1 and 002.2 | `internal/agent/runtime/agentctl/launcher` (`childEnvOverrides`) |

## Probe stderr capture

The probe executor attaches a trailing-window buffer to the child's stderr.
The buffer keeps only the last `stderrTailLimit` (4 KiB) bytes: `Write` discards
overflow from the front, so a chatty or looping child cannot grow the capture
or the log entry that carries it. An empty tail is itself a result; it means
the child produced no diagnostics, not that diagnostics were lost.

## Classify on raw stderr, sanitize for the log

Failure classification consumes the raw tail, because marker matching
(`managedRuntimeProbeFailureCode`, the authentication markers of
`sanitizeProfileProbeFailure`) and the upstream status extraction of
`withUpstreamHint` are defined over the child's actual output. The upstream
hint only extracts an HTTP-status-shaped match, so no credential can reach a
client message through it.

The log boundary applies the opposite rule. `sanitizeProbeDiagnostic` scrubs temporary
paths, then applies `securityutil.Sanitize`, the existing provider-diagnostic
redaction tier. It masks prefixed and opaque token-shaped values, strips URL
paths, queries and fragments, and redacts local paths. Error codes and ordinary
failure descriptions remain available. The credential-only tier is intended
for user-authored text and does not protect these provider diagnostics.

`stderrBuffer` records whether its bounded raw capture starts inside a line.
The raw `tail()` remains available for managed-runtime classification. Its
separate `probeTails()` drops the first retained line when the cut fell inside
that line, before sanitizing: it can be a credential suffix whose key or
prefix fell outside the window. A cut immediately after a newline preserves
the following complete diagnostic line. With no retained newline after a cut
inside a line, the log tail is empty. All later complete lines remain eligible for logging, within the same 4 KiB bound.

The direct failure's structured `error` field and response error also use the
provider-diagnostic sanitizer, so the HTTP handler's repeated error log cannot
bypass redaction. Secondary OpenCode model-discovery errors use it too; those
errors can embed captured child stderr. Classification and profile response
selection still consume the raw evidence.

Both probe failure sites (`ACP probe failed` and `ACP profile probe failed`)
include the sanitized underlying error, sanitized tail and managed failure
code in the message, as well as structured diagnostic fields. The JSON relay
retains only its trusted envelope and message, so safe diagnostic text must be
present in that message to remain useful after projection. Arbitrary structured
fields stay excluded. The profile site retains the pre-sanitization managed
classification because its response deliberately replaces the message and code.

The sanitizer implementation and its existing regression suite belong to the
neutral `internal/common/securityutil` package. Runtime routing keeps its
existing exported functions as forwarding wrappers; agentctl imports the shared
package directly. This avoids a new dependency on backend runtime ownership.

The client response never carries the stderr tail. Profile responses remain
generic; direct protocol errors retain their diagnostic wording after
redaction. The log is the operator's window into the child-side stderr cause.

## Launcher-owned child log format

When the agentctl child writes to inherited stdout, it feeds the supervising
launcher's relay. Agent-survival file-sink selection is unchanged. The relay
recognizes console and JSON envelopes, but only JSON applies the structured-field
allowlist. The managed child format is the launcher's contract:
`childEnvOverrides` pins `AGENTCTL_LOG_FORMAT` to the JSON format the relay
parses. Because the pin is an environment override,
any inherited value of that variable from the operator's ambient environment
is dropped rather than merged. Standalone agentctl processes keep their own
`AGENTCTL_LOG_FORMAT` configuration.

## Verification

Regression tests cover each criterion at the owning boundary:

- `TestChildEnvOverridesPinRelayParseableLogFormat`
  (`internal/agent/runtime/agentctl/launcher`)
- `TestProfileProbeFailureLogCarriesScrubbedStderr` and
  `TestProfileProbeFailureLogMasksCredentialShapedStderr`
  (`internal/agentctl/server/utility`, profile context)
- `TestProbeFailureLogMasksCredentialShapedStderr`
  (`internal/agentctl/server/utility`, direct probe context)
- `TestProbeFailureLogRedactsProviderDiagnostics`,
  `TestProbeFailureLogDiscardsTruncatedCredential`,
  `TestProbeFailureLogDiscardsTruncatedUnterminatedLine`, and
  `TestProbeModelDiscoveryFailureLogRedactsProviderError`,
  `TestProfileProbeFailureLogRetainsProtocolErrorWithoutStderr`, and
  `TestProbeFailureLogKeepsDiagnosticsInRelayMessage`
  (`internal/agentctl/server/utility`, provider privacy and truncation)
- `TestStderrBufferTailKeepsTheEnd` and `TestStderrBufferBoundsRetentionOnWrite`
  (`internal/agentctl/server/utility`, bounded capture)
