# Real Pi and Greywall reproduction

Tests: 2026-10-14 through 2026-10-15. Platform: Ubuntu 24.04, Linux 6.8, x86_64.

This is diagnostic evidence, not a supported configuration or the proposed permanent fix.
The exact-`greywall` build overlay was rejected as the implementation direction.
See [the generic design handoff](plan.md).

## Installed programs

- Pi: `@earendil-works/pi-coding-agent@1.2.0`.
- ACP adapter: `pi-acp@0.0.34`, matching Kandev's pinned version.
- Greywall: `0.3.7`. Release archive checksum verified.
- Node: `24.12.0`. Bubblewrap: Ubuntu package `0.9.0-1ubuntu0.3`, extracted locally.
- Greyproxy: `0.4.5`, checksum verified. It was not started for the initial handshake tests.

Tools and evidence were retained in a private host-local diagnostic directory.
The local agent handoff records its location; artifacts are not included in this PR.
The installation did not change global npm packages, system packages, or services.
Child processes received a minimal environment and a disposable home.
A synthetic OpenAI key in that home enabled local model discovery.
The initial handshake phase used no real credentials or model prompts.
The later full-session and browser phases used a private copy of an existing login access token.

## Results

1. Direct `pi-acp`: initialize passed. Without the synthetic key, session/new returned the expected authentication-required response.
   With the synthetic key, session/new returned 44 models and 5 modes.
2. Greywall around `pi-acp`: the same handshake passed after granting access to the local installation and Pi state.
3. Direct managed command: `npx --yes --prefer-offline --prefix <private-temp-directory> pi-acp@0.0.34` passed.
   This populated only the disposable npm cache before the sandbox run.
4. Greywall around that managed command: initialization failed with npm `ENOENT` for the prefix directory.
   Adding `--allow-path <private-temp-directory>` made it pass from a normal workspace.
5. With a working directory under `/tmp`, initialization passed but session/new failed.
   Pi reported that its executable was missing. Adding `--allow-path .` made the real Kandev probe pass.

The final test used `agents.NewPiACP().BuildCommand` and `ACPInferenceExecutor.Probe`, with `ProfileContext: true`.
It used the real managed npm marker and a temporary probe working directory.
The source first rejected `greywall` with `unsupported_profile_context`.
A Go overlay that added only an exact `greywall` resolver case exposed the filesystem failures.
With that overlay and both filesystem grants, the real probe returned 44 models in approximately four seconds.

## Causes

Kandev's utility resolver excludes Greywall even though its ordinary ACP launch builder accepts the prefix.
This blocks profile discovery before Greywall executes.

Greywall's Linux sandbox replaces `/tmp` with a private filesystem.
Kandev provisions its isolated npm project under the host temporary root.
The host-utility manager also creates its probe workspace there.
Neither host directory is automatically visible to the sandboxed adapter.
The npm failure occurs before ACP initialize.
The hidden probe workspace fails later, when pi-acp spawns Pi with that workspace as `cwd`.
The adapter labels the resulting `ENOENT` as a missing executable, which obscures the missing directory.

This behavior is independent of provider credentials or a model request.
The first diagnostic installation under `/tmp` was also hidden, so it was moved outside `/tmp` before continuing.

## Working prefix shape

With the temporary resolver overlay, the tested prefix has this shape:

```text
greywall --allow-path . --allow-path <resolved-managed-npm-project-directory> --
```

This is a shape, not a paste-ready command. Replace the placeholder with the execution host's exact directory.
The diagnostic test obtained that directory from `managedruntime.PrepareNPMProjectPrefix`.
Its default basename is `kandev-managed-npm-runtime-<user-scope>` under the process temporary root.
The test also granted read access to its isolated Node/npm/Pi installation and write access to its disposable `.pi` and `.npm` directories.
An existing installation's Greywall profile may already provide those tool and state grants.
Do not infer that it provides the two Kandev temporary-directory grants.
No `--watch`, learning mode, allow-all network mode, or whole-`/tmp` grant was used.

## Evidence

Files under the retained diagnostic directory:

- `logs/direct-placeholder.jsonl` and `logs/greywall-home.jsonl`: successful direct and wrapped handshakes.
- `logs/npx-greywall.jsonl`: npm's missing-prefix error.
- `logs/tmp-workdir.jsonl`: session/new's misleading missing-Pi error.
- `logs/kandev-current.txt`, `logs/kandev-overlay.txt`, and `logs/kandev-overlay-mounted.txt`: real probe failure and success.
- `logs/repro_real_greywall_test.go.txt`: the throwaway Go test used for the final result.

The initial temporary repository tests and overlay were removed after that phase.
A later isolated browser instance was built with a separate overlay and left running
for the requested manual tests; see the browser phase below.
The installed programs and raw protocol evidence remain available for follow-up work.

## Full prompt and tool test

A subsequent test completed a real provider round trip and three tool calls inside Greywall.
It used the existing Codex login through Pi's `openai-codex` provider.
Only a valid access token was copied into the private diagnostic home. No refresh token was copied.
The observed model was `openai-codex/gpt-6.1-sol`, with low reasoning effort.
Pi selected this available model at startup. The initially requested legacy model was unavailable.

Both direct and sandboxed workspaces contained `input.txt` with seed 413, multiplier 7, and offset 19.
The prompt required Pi to read those values, calculate `seed * multiplier + offset` with bash,
write `result.txt`, read it back, and reply with the verified value.

Both runs completed the same three tool operations: read, execute, and read.
Both produced the exact bytes `2910\n` in `result.txt` and replied `VERIFIED:2910`.
The protocol trace contains completed tool updates and a successful prompt response.
This verifies a real model response and filesystem effects, not only startup or model discovery.

The sandboxed command used the managed `npx` form with the exact npm-directory grant and `--allow-path .`.
An owned Greyproxy process ran in the foreground with its disposable database.
The Greywall command added only `--allow chatgpt.com:443` for provider network access.
It did not enable watch mode, learning mode, or an allow-all network policy.
The command also granted access to the diagnostic tool installation, npm cache, and private Pi state directory.

Additional evidence under the retained diagnostic directory:

- `full-session/sandbox-command.txt`: exact wrapped command, without credentials.
- `full-session/logs/direct.jsonl` and `full-session/logs/sandbox.jsonl`: real prompt and tool frames.
- `full-session/logs/verified-results.json`: independently checked file contents and completed tool counts.
- `full-session/direct/result.txt` and `full-session/sandbox/result.txt`: resulting files.

The full-session test access-token copy was removed after that phase.
Its proxy process was stopped and its disposable database removed.
The later browser instance used a separate private access-token copy and proxy.
The original Codex login was not modified.

The ACP debug CLI attempts the legacy `session/set_model` method when given `--model`.
Pi rejected that method. The successful runs selected their model through Pi's private settings before startup.
This diagnostic-tool limitation did not require a production Kandev change.

## Kandev browser instance

A separate isolated Kandev instance was built with the same diagnostic resolver change.
The build used a Go overlay; repository production source remained unchanged.
Its separate database contained a disposable repository and a Pi + Greywall profile.
The profile included explicit installation, cache, Pi state, managed npm, and working
directory grants. These absolute paths were test configuration, not product defaults.

Profile model discovery returned `status=ok`. A normal ACP task, with passthrough disabled,
completed three tool calls, wrote the expected result, and replied `VERIFIED:2910`.
The session reached `WAITING_FOR_INPUT`. Browser inspection confirmed the reply and composer.
This extends the earlier command-line evidence to Kandev's normal chat delivery.
It does not verify Kandev MCP operations; the session reported unsupported MCP transports.

The instance was left running for manual use. Its original login was not modified,
and no refresh token was copied. Before reusing it, check process ownership, token
expiry, and the private instance record. Do not publish authentication files or raw
protocol traces. The local handoff records teardown commands and the separate token copy.

## Remote frontend loading

Slow access to the test instance was separate from Pi/Greywall compatibility.
A Tailscale HTTPS endpoint and same-origin API routing did not resolve the user's delay.
A browser test with 150 ms latency, 10 Mbps download, 5 Mbps upload, and cache disabled
measured 79.435 seconds and 3,142 requests before the development frontend showed the reply.

Replacing Vite dev with a bundled frontend served through Vite preview reduced the same
measurement to 8.711 seconds and 157 requests. Compressed asset responses were verified.
The direct HTTP page and API on the Tailscale IP also returned HTTP 200 with the bundled
assets. The user confirmed access through that direct address. HTTPS is not required
for this test setup. These measurements are simulated-network checks, not a measurement
of the user's connection or a general frontend benchmark.

## Remaining limits

Real Linux tests covered startup, capability discovery, provider inference, file tools,
and Kandev chat delivery only with the temporary overlay and explicit test policy.
They do not establish a permanent generic fix, every launch path, all providers,
MCP operations, policy enforcement against bypass, or macOS behavior.
The original reporter's exact configuration is still unknown.

The next design must address both inconsistent wrapper admission and runtime context.
Do not convert the diagnostic exception or its absolute path grants into production rules.
