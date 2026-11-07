# OpenCode v2 integration evidence

## Evidence boundary

This document records source facts, packaged conformance results, and remaining
evidence gaps. It contains no live prompts, callback credentials, private
database rows, or full tool output.

Source baseline: OpenCode 2.2.0, commit
`1097e2668b1e08ad63bdf0b1c4860b9bc01e420c`. Existing Kandev investigation confirmed
completed native shell results while retained ACP notifications lacked their
job outcomes. Retained notification captures are not complete JSON-RPC wire logs.

## Source facts

| Surface | Verified source fact | Limit |
| --- | --- | --- |
| `packages/plugin/src/promise/plugin.ts` | V2 plugin has `id`, `setup(ctx)`, and optional cleanup | No v1 hook-export compatibility claim |
| `services/www/src/docs/content/build/plugins/index.mdx` | `plugins` accepts local directories and entry options | Actual merge and loader behavior need packaged proof |
| `packages/plugin/src/promise/event.ts` | Plugin context exposes filtered native event subscription | Connection/readiness behavior needs end-to-end proof |
| `packages/client/src/shared-events.ts` | Connection events, cancellation, bounded subscriber queue, and activity callback exist | Do not equate transport health with shell liveness |
| `packages/schema/src/shell.ts` | Created/exited/deleted events expose shell IDs and real status | Exit code and timestamps can be absent |
| `packages/core/src/tool/plugin/shell.ts` | Created shell metadata carries native session ID | Background intent is not guaranteed there |
| `packages/core/src/config.ts` | Inline configuration uses `OPENCODE_CONFIG_CONTENT` | Preserve all existing precedence and substitution semantics |
| `packages/cli/src/commands/handlers/acp.ts` | ACP starts its own private authenticated server | Do not poll a different service |
| `packages/core/src/shell.ts` | Native status retention has a process-local cap of 25 exited shells | Missing status is not successful completion |

Source URLs use the baseline commit under
`https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/`.
The system design links the principal source files directly.

## Upstream tracking, checked 2026-10-28

- [Issue 55760](https://github.com/anomalyco/opencode/issues/55760) is open and
  marked reproduced. Its
  [shell reproduction](https://github.com/anomalyco/opencode/issues/55760#issuecomment-6277602640)
  identifies missing shell projection and automatic parent updates.
- [PR 55834](https://github.com/anomalyco/opencode/pull/55834) is open and
  unmerged. Its patch retains a child-count shutdown gate and does not translate
  native shell outcomes. Reviews also raise event-loss and turn-ownership risks.
- [PR 53211](https://github.com/anomalyco/opencode/pull/53211) is open and
  unmerged. It changes waiting/terminal session semantics.
- [PR 56849](https://github.com/anomalyco/opencode/pull/56849) is an open,
  unmerged experimental ACP v2 preview. It is not compatibility proof for the
  current Kandev adapter.

Recheck upstream state before implementation and source-authority selection.
Do not infer shipped support from an issue title, closed state, or version alone.

## Required Task 01 record

Record these fields after the actual packaged test succeeds:

- Exact CLI package/version, installation method, source commit, and binary hash.
- Plugin source revision, artifact hash, protocol version, and behavior versions.
- Real plugin export loading and cleanup, without a workspace-linked substitute.
- Native subscription readiness before the first test job.
- Preservation of file configuration, inline JSONC, plugin order/options, and
  configuration substitutions. Include a reserved-ID conflict.
- Actual background launch, native terminal event, and the corresponding bridge
  envelope with synthetic identities and no sensitive content.
- A job ending before claim delivery and a job ending after the foreground reply.
- Repeated launch, native-v1 negative case, incompatible-v2 case, and disabled case.
- Whether each result uses the real CLI, a local model stub, or a fake context.

## Required delivery and product record

Task 03 records commit-before-ack, lost-ack retry, original-turn attribution,
duplicate suppression, gap handling, and no root completion.

Task 05 records separate deterministic browser, real packaged browser, Docker,
SSH, and Kubernetes results. Include exact commands and limitations. A callback
posted by a mock provider is not evidence that OpenCode loaded the real plugin.

Until those records exist, all compatibility and delivery claims remain planned.

## Task 01 packaged conformance record

Run on 2026-10-28 and repeated on 2026-10-29 in an isolated temporary HOME, XDG
roots, workspace, OpenCode database, and npm cache.

| Field | Recorded result |
| --- | --- |
| CLI package | `@opencode/cli@2.2.0`, installed by npm for this test only |
| Reported CLI version | `opencode v2.2.0` |
| Source baseline | OpenCode commit `1097e2668b1e08ad63bdf0b1c4860b9bc01e420c` |
| Plugin artifact | `assets/index.mjs`, 29265 bytes |
| Artifact SHA-256 | `893f9535bd6216d66b2a4535b4d9beafa19351f4d2e945167e168bf25b4f4c30` |
| Protocol behaviors | `shell-outcomes` version 1 |
| Command | `KANDEV_OPENCODE_CONFORMANCE=1 pnpm --filter @kandev/opencode-plugin test:conformance` |
| Result | Passed: shipped artifact loaded through additive `OPENCODE_CONFIG_CONTENT`, a file-configured user plugin retained its options, hello carried the exact artifact hash and CLI version, a confirmed shell claim produced one `shell.terminal` observation with exit code 3, and no duplicate followed |

The harness also runs `opencode debug config --standalone` against the same
isolated configuration. It reports `username: "alice"` from
`{env:KANDEV_CONFORMANCE_USERNAME}` and retains the file-configured user plugin,
so environment substitution and the file configuration layer survive launch
composition.

The conformance test rejects any binary that does not report exactly
`opencode v2.2.0`. It never uses the developer's managed runtime selection,
native installation, credentials, or global configuration.

## Task 03 and Task 04 packaged agentctl record

`TestOpenCodePluginPackagedShellOutcome` (`-tags e2e`, opt-in with
`KANDEV_OPENCODE_REAL_E2E=1`) installs `@opencode/cli@2.2.0`, starts the real
agentctl process manager with `features.opencodePlugin` enabled, and uses the
real embedded artifact, private host, ACP launch claim, callback, durable
journal, and ordered delivery writer. The local model stub emits one `shell`
tool call with `background: true` and `exit 3`.

Recorded result on 2026-10-29:

| Evidence | Result |
| --- | --- |
| Host hello | Accepted for `shell-outcomes` version 1 |
| ACP launch claim | One claim for the native shell identity |
| Callback | One `shell.terminal` observation received |
| Durable boundary | One `background_work_updated` journal event |
| Projection | `failed`, exit 3, original source call and title retained |
| Capabilities | `events_only`, output `none`, no actions |

The event reaches the existing journal and workload projection only. The test
asserts no root completion, conversation message, task change, workflow move,
or usage effect.

## Remaining evidence gaps

- Dedicated bridge-enabled desktop and phone browser scenarios using the real
  plugin receiver are not added yet. The existing desktop and phone
  background-work suites continue to cover the flag-off unknown fallback.
- Docker, SSH, and Kubernetes executor proof was not run in this implementation
  turn. Artifact staging is embedded in every agentctl variant, but the
  executor-local callback path needs its own recorded run.
- No historical-job backfill, live output, process controls, automatic assistant
  follow-up, or active-background liveness is claimed.

## Review remediation record

A read-only review raised five findings on 2026-10-29. The implementation now
addresses them as follows.

| Finding | Remediation and evidence |
| --- | --- |
| Retry backoff was not awaited | The outbox now owns one awaited, abortable backoff for the whole failed queue. A newer enqueue cannot bypass it, timers cannot overlap, and exhaustion remains a bounded gap. `test/outbox.test.mjs` covers repeated fast failures, recovery, an enqueue during backoff, and cancellation. |
| Revocation was not fenced against the durable write | The host uses a dedicated admission fence across behavior admission, the committer call, and claim retirement. A revocation or close advances an admission epoch before waiting for the fence, and the manager rechecks that epoch immediately before durable admission. The ACP adapter also provides a native-session read guard held across the journal commit, so a session transition or reset cannot revoke ownership between validation and persistence. The manager's commit path is cancellable and never acknowledges a cancelled write. Host and process tests cover revocation-before-commit, commit-before-revocation, close during commit, stale generation, and cancellation. |
| Completed jobs exhausted the 256-claim capacity | Successful durable acceptance retires the exact claim in the host and adapter. Receipts remain for lost-ACK retries, and hash-checked duplicates are accepted without another projection. Tests cover more than 256 sequential completions, live-capacity bounds, and duplicate/conflicting deliveries after retirement. |
| The shared core required shell claims | Behavior configuration is now behavior-owned `data`; each behavior decodes its own config, admits its own payload, and supplies its stable observation key. The host and plugin core contain no shell claim branch. A test-only non-shell behavior negotiates and delivers through both the actual core and the host. |
| Cached artifact validation accepted modified package/manifest files and symlinks | Staging now verifies all three reviewed files byte-for-byte against the embedded artifact, rejects symlinked files and directories, and re-stages inconsistent content. Tests cover entrypoint, package-manifest, artifact-manifest, file-symlink, and directory-symlink regressions. |

Work order 05 remains in progress. These fixes do not add bridge-enabled
desktop/phone browser proof or Docker, SSH, and Kubernetes executor proof.
