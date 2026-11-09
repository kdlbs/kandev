# OpenCode continuity evidence

## Investigation baseline

Read-only investigation on 2026-10-27. Kandev revision:
0b1017d81547f44c6dddaf52ed1471767e499c1b.
OpenCode package: @opencode/cli 2.2.0.
Upstream tag commit: 1097e2668b1e08ad63bdf0b1c4860b9bc01e420c.

No raw prompts, credentials, full tool results, private database rows, or
machine-specific output paths are copied into this package. The findings
support planning; they are not new producer conformance results.

| Observation | Result |
| --- | --- |
| ACP notification capture | 762 decoded notifications; matching normalized kinds, tool IDs, text, and reasoning |
| Background launches | Three shell inputs used background=true |
| Launch results | Each returned metadata.shellID with metadata.status=running |
| Kandev projection | Three completed launch calls; no workload or run rows |
| Native continuation | A shell result and automatic assistant message existed after the last captured ACP notification |
| Raw-log limit | The files log SDK-decoded session notifications; they do not prove every JSON-RPC message reached the SDK |

The three jobs were local E2E reproduction, a PR await script, and a five-minute
CI polling script. The local reproduction completed with 31 passing tests.
The poller wrote subsequent samples while Kandev showed the foreground idle.
These historical results do not establish current CI or process state.

## Primary producer sources

- [ACP prompt subscription](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/cli/src/acp/turn.ts#L295).
- [Tool result metadata](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/cli/src/acp/tool.ts#L100).
- [Native synthetic and shell events](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/schema/src/session-event.ts#L285).
- [Existing wire-test pattern](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/cli/test/acp/turn.test.ts).

## Task 02 producer gate

Pending. Record all fields before marking producer proof complete:
- Upstream repository, commit, accepted contract version, package version,
  build/install command, and binary digest.
- Raw JSON-RPC handshake and capability agreement.
- Exact tool input, detached result, root prompt response, later job outcome,
  automatic cycle start/content/end, and next ordinary prompt.
- No duplicate content during prompt/session subscription overlap.
- Atomic expected-idle-revision admission, definite not-accepted rejection, and
  uncertain-response behavior when automatic execution races a user message.
- Close, reset, disabled opt-in, unsupported peer, reconnect/snapshot, and
  permission ownership results.
- Which evidence uses a fake server, a real packaged CLI, or a real model.
- Recovery and output limits. Mark unsupported controls explicitly.

Keep a sanitized fixture with stable synthetic IDs. Do not commit a fresh
live-session transcript. A consumer test constructed from the proposed contract
does not satisfy this upstream gate.

## Kandev implementation evidence (2026-10-27)

No upstream producer artifact exists in this workspace. The following evidence
is consumer-side and deterministic; it does not satisfy the Task 02 gate.

| Evidence | Scope | Limit |
| --- | --- | --- |
| `opencode_background_test.go` adapter suites | Captured OpenCode 2.2.0 launch frames through the real normalizer, adapter, and tool-update path | Synthetic frames, not a packaged CLI |
| `/opencode-background-fixture` mock producer | Three identified launches through the real `opencode-acp` adapter identity registered via `KANDEV_MOCK_PROVIDERS` | Fake producer, real consumer stack |
| Desktop and phone Playwright suites | Chip, unknown counts, unavailable output, absent controls, history, prompt admission | No automatic follow-up assertion |

The recorded 2.2.0 behavior remains: the launch is confirmed and inspectable,
then settles to `unknown` because no later outcome can arrive on the prompt
subscription. Any future producer evidence must replace, not extend, this table
with the fields listed above.
