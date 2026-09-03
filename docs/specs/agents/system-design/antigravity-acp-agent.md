---
status: draft
system: agents
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-002
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-005
  - REQ-AGENTS-ANTIGRAVITY-ACP-006
  - REQ-AGENTS-ANTIGRAVITY-ACP-007
  - REQ-AGENTS-ANTIGRAVITY-ACP-008
---

# Google Antigravity ACP agent system design

## Purpose and boundaries

The agent system owns the Antigravity agent's declared identity, discovery
predicate, launch argument vector, config-root template, skill directories, and
remote-auth methods. Google's ACP server owns its own credentials, session
storage, harness subprocess, and model catalogue.

This design does not own ACP transport, process supervision, capability
probing, MCP wiring, or executor provisioning. It constrains only what Kandev
declares about this agent and what Kandev may assume about the server.

Six requirement criteria touch those unowned layers and are deliberately
written as *declare-nothing-new* constraints rather than as new behavior:
`AC-AGENTS-ANTIGRAVITY-ACP-003.13` on the shared command builder's `cli_flags`
and sandbox-prefix append, `AC-AGENTS-ANTIGRAVITY-ACP-005.1` and `.2` on the
advertised authentication methods and their order,
`AC-AGENTS-ANTIGRAVITY-ACP-005.3` and `.4` on authentication and stderr, and
`AC-AGENTS-ANTIGRAVITY-ACP-006.1` on capability derivation. Each is satisfied by
this agent adding no agent-specific override, because the shared layer already
behaves that way: `CommandBuilder.BuildCommand` appends `cli_flags` for every
agent with no per-agent opt-out (see [launch surface](#launch-surface)), the
session-start adapter caches the `initialize` result's `authMethods` and
re-emits them in that order, `maybeEmitAuthRequired` classifies a `-32000` as
auth-required whenever auth methods are cached, and the capability path reads
`agentCapabilities`. Build should assert the absence of an override, not
re-implement the shared rule. If a future change makes the shared rule
insufficient for this agent, that is a change to the runtime's design, not this
one.

**Which layer presents the auth methods.** `AC-AGENTS-ANTIGRAVITY-ACP-005.1` and
`.2` bind the session-start adapter: it is the layer that caches the advertised
methods when `initialize` returns and re-emits them, in the server's order, when
`session/new` answers `-32000`. They do not bind the capability probe: on an
authenticated run that path does read the advertised methods, but when
`session/new` answers `-32000` it returns before reaching that step. An
unauthenticated probe therefore records the auth-required state of
`AC-AGENTS-ANTIGRAVITY-ACP-005.3` carrying no method list, which is the shared
behavior for every ACP agent rather than something this one declares. Making the
probe carry them would change a shared cross-agent path this design does not
own, so it is out of scope here rather than implied by `.1`.

Values here are either **measured** against build
`agy_acp_server_20260818_01_RC01`, darwin-arm64, on 2026-09-03, or **registry**
values read from `antigravity-acp/agent.json` and never executed; each section
says which. The probed artifact was removed after the session and no transcript
was retained, so the measured values are a summary of that session and
re-checking one means re-downloading that build. Only darwin-arm64 was ever
executed: every Windows and Linux value here is registry-sourced. This is a
record of an external contract, not a Kandev implementation plan.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-ANTIGRAVITY-ACP-001` | [Distribution artifact](#distribution-artifact) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-002` | [Harness resolution](#harness-resolution) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-003` | [Launch surface](#launch-surface) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-004` | [Persistence](#persistence) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-005` | [Security](#security) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-006` | [Data and contracts](#data-and-contracts) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-007` | [Distribution artifact](#distribution-artifact) |
| `REQ-AGENTS-ANTIGRAVITY-ACP-008` | [Control flow](#control-flow) |

## Components and responsibilities

- **Kandev agent declaration.** Identity, discovery, argument vector, config
  root, skill directories, remote-auth methods. Static metadata only.
- **`agy_acp_server.par` / `.exe`.** Google's ACP server. A Mach-O executable
  wrapping a packed Python archive; the `.par` suffix is part of the filename,
  not an interpreter hint, and the file is executed directly. Speaks ACP over
  stdio.
- **`localharness_external`.** A sibling executable the server drives over gRPC
  using protobuf messages (`localharness_pb2`). Located by the server itself,
  never by Kandev.

### Distribution artifact

The ACP registry entry `antigravity-acp/agent.json` publishes a
`distribution.binary` matrix, read verbatim on 2026-09-03:

| Platform | `cmd` | `args` |
| --- | --- | --- |
| `darwin-aarch64` | `./agy_acp_server.par` | none |
| `linux-x86_64` | `./agy_acp_server.par` | `["--uid="]` |
| `linux-aarch64` | `./agy_acp_server.par` | `["--uid="]` |
| `windows-x86_64` | `./agy_acp_server.exe` | none |
| `windows-aarch64` | `./agy_acp_server.exe` | none |

**The executable name carries its suffix on every platform.** There is no
`agy_acp_server` without an extension: the archives contain `.par` (darwin,
linux) and `.exe` (windows), and the registry invokes exactly those. Kandev's
discovery name and argument vector therefore use the suffixed, platform-specific
name and require no rename, symlink, or wrapper from the user. An earlier draft
of this pair used the bare name and would have made the agent undiscoverable on
every platform.

The darwin-arm64 archive is 314,500,221 bytes,
`sha256 f122ca7e7030a27f9649da4cf1a7d80e12c48c5f6118ff35affc34d56cbf83dd`. It
holds exactly two archive-root entries:

| Entry | Uncompressed | Mode |
| --- | --- | --- |
| `agy_acp_server.par` | 792,105,680 | `0755` |
| `localharness_external` | 101,551,680 | `0555` |

An extraction therefore costs roughly 894 MB. The executable is signed
`Developer ID Application: Google LLC (EQHXZ8M8AV)` with hardened runtime and a
2026-08-19 timestamp, so it runs on macOS without a quarantine prompt when
fetched by a non-quarantining downloader.

The archive URL carries a rotating build stamp
(`agy_acp_server_20260818_01_RC01`) and Google publishes no checksum beside it.
That, the size, and the fact that Kandev executes `InstallScript()` verbatim as
remote shell are the three reasons provisioning is excluded from this layer.

`agy` 1.1.25, the separate Antigravity CLI, is not an alternative entry point:
it exposes no ACP mode in `--help` or `--helpfull`, contains no reference to the
ACP server, and authenticates into `~/.gemini/antigravity-cli/`.

### Prior-art departure

Kandev's built-in ACP agents that launch a bare binary rather than `npx` are
Cursor, Kimi, Kiro, Qoder, Trae, Devin, Grok, Hermes and Omp. Each detects one
PATH entry and stops, and each returns a non-empty `InstallScript()` — Kimi,
Kiro and Qoder return prose rather than runnable shell, which is the latent form
of the hazard below, while Cursor, Devin and Hermes return real installers. This
agent departs on three counts, each forced by measured behavior rather than
preference.

**Discovery checks a second file.** With the executable present but its harness
absent, the server still starts and still answers `initialize`, logging only
`Localharness not found.` to stderr. One-entry detection would therefore report
a half-installed agent as healthy and lose capability later, inside a prompt.
The predicate checks the sibling directly so the failure surfaces at discovery.
See [harness resolution](#harness-resolution) for the search order this mirrors.

**No CLI passthrough.** Every one of those agents also declares a passthrough
config so the user can drop to the raw CLI under a PTY. This server has no such
mode: `--helpfull` exposes only `--[no]debug` and `--[no]notices`, there is no
subcommand or REPL, and the process speaks ACP over stdio and exits on stdin
EOF. Declaring passthrough would surface a toggle that spawns a process the user
cannot interact with, so AC-AGENTS-ANTIGRAVITY-ACP-001.9 declines it.

**The install script is empty.** That value runs verbatim as remote shell on the
SSH, Kubernetes and Sprites executors, so prose in it is a command those
executors attempt to execute. Because the artifact is also a version-pinned
314 MB download expanding to roughly 894 MB, the honest value is the empty
string, and the install guidance moves to `Description()`.

## Data and contracts

`--helpfull` exposes exactly two flags on darwin, `--[no]debug` and
`--[no]notices`. There is no `--uid`, `--model`, `--acp`, or port flag, which is
why the argument vector cannot vary with model, permission policy, or resume
target.

`initialize` returns:

- `agentInfo`: `{name: antigravity-acp, title: Google Antigravity, version:
  agy_acp_server_20260818_01_RC01}`
- `agentCapabilities`: `loadSession: true`; `promptCapabilities` `{image, audio,
  embeddedContext}`; `mcpCapabilities` `{http: true, sse: true}`;
  `sessionCapabilities` `{list, resume}`; `auth: {logout}`
- `authMethods`: `oauth-personal`, `oauth-business`, `gemini-api-key`,
  `agent-platform`

The server echoes the requested `protocolVersion` verbatim and does not clamp:
a request of `1` returned `1`, and a request of `99` returned `99`. The echoed
value carries no information about what the server supports, which is why
capability decisions must read `agentCapabilities` instead.

## Launch surface

One argument vector serves the session command, the runtime command, and the
one-shot inference command, so no path can drift to a different one:

| Platform | Argument vector | Source |
| --- | --- | --- |
| darwin | `agy_acp_server.par` | registry `cmd`; binary measured |
| linux | `agy_acp_server.par --uid=` | registry `cmd` + `args`, not measured |
| windows | `agy_acp_server.exe` | registry `cmd`, not measured |

The name is the same one discovery looks up on that platform, which is what
keeps a successful detection from producing an unlaunchable command. Selection
is by the platform of the Kandev process building the command — the host, not an
executor's target. That distinction is real rather than theoretical: discovery
calls `exec.LookPath` on the host, while the SSH executor ships the built command
verbatim to the remote, so a darwin host driving a hand-provisioned linux remote
would send the darwin vector and drop `--uid=`. Nothing here can correct it,
because command construction receives no per-executor platform, and the
requirements name the mixed-platform remote as an explicit exclusion.

The rules are also total: every platform yields a defined lookup name and a
non-empty vector. There is no "build no command" outcome to express — the command
surfaces return a value, not an error — and an empty vector would not be inert,
because the SSH preflight skips binary verification when the built command has no
arguments, turning a clean early failure into an opaque spawn error later.
Availability is gated by discovery alone.

Nothing else varies the declared vector. `--helpfull` exposes only `--[no]debug`
and `--[no]notices` on darwin, so there is no model, permission, auto-approve,
agent-type, or resume flag to vary, and this agent's own construction passes
neither of the two that exist. The working directory is the task workspace, the
environment map is empty with nothing stripped, and shutdown needs no forced kill
because the server exits on stdin EOF.

**Which layer the vector describes.** Everything above describes what this agent
*constructs*, which is not the whole of what is launched.
`CommandBuilder.BuildCommand` calls the agent's own `BuildCommand` and then
appends the profile's configured `cli_flags` to every agent's vector, after which
`prependCommandPrefix` prepends any sandbox launcher tokens. No agent has an
opt-out from either, and adding one here would change a contract shared by all of
them, which [purpose and boundaries](#purpose-and-boundaries) rules out. So
AC-AGENTS-ANTIGRAVITY-ACP-003.7 binds this agent's own construction only: a user
who puts `--debug` or `--notices` in their profile's `cli_flags` gets it passed,
and that is the shared `cli_flags` feature working as designed rather than a
defect in this agent. The same boundary is why the "exactly" in
AC-AGENTS-ANTIGRAVITY-ACP-003.1 and `.2`, and the list in `.6`, are claims about
the declared vector rather than the final argv.
AC-AGENTS-ANTIGRAVITY-ACP-003.13 states that binding, so a test asserting it
knows which layer to call.

The linux `--uid=` argument is the one real risk in this table. The darwin build
exposes no `uid` flag at all, and absl rejects an unknown flag at startup, so if
the linux build matches darwin rather than the registry the agent fails to start
there. That argument stays registry-sourced and unverified: the requirements
exclude Windows and Linux verification from this layer, so confirming it with
`--helpfull` against the linux archive is follow-up work, not a condition of
this change.

## Control flow

Kandev spawns one server process per session and speaks ACP over its stdio. The
server spawns and supervises its own harness subprocess.

Concurrent sessions run independent processes over a shared config root. Kandev
adds no lock: arbitration inside the config root is the server's own concern.

Closing stdin after `initialize` terminated the process within one second with
exit code 0, so graceful shutdown needs no forced process kill.

## Failure and recovery

### Harness resolution

This is the design's central hazard. The server resolves its harness like this:

1. If `ANTIGRAVITY_HARNESS_PATH` is already set, return immediately.
2. Otherwise search `dirname(abspath(sys.argv[0]))`, then
   `dirname(abspath(sys.executable))`, for `localharness_external` then
   `localharness` (`.exe` variants on Windows), first match wins.
3. If nothing matches, log `Localharness not found.` and continue starting.

Kandev accepts a candidate only when it is a regular file. On non-Windows
hosts, it also requires at least one execute bit. Windows mode bits do not
control executable access, so the regular-file check applies there.

`abspath` does not resolve symlinks, and step 3 is not fatal. Measured:

| Layout | Result |
| --- | --- |
| Real path, harness sibling present | clean start, no warning |
| Real path, harness removed | `Localharness not found.`, still starts |
| Symlink from a harness-less PATH directory | `Localharness not found.`, still starts |

In all three cases `initialize` succeeded. A user who symlinks the executable
onto PATH, or extracts only one of the two files, therefore gets an agent that
looks healthy at every point Kandev normally checks and loses capability later,
inside a prompt. Kandev's discovery predicate closes that gap by checking the
sibling directly, and treats a set `ANTIGRAVITY_HARNESS_PATH` as satisfying it
because step 1 makes the directory irrelevant in that case.

### Authentication errors

`session/new` before authentication returns JSON-RPC `-32000` `"Authentication
required"`, with a message enumerating the settings key and the environment
variables each method needs. The server also writes a Python traceback to stderr
for that handled error, so stderr traceback text is not evidence of a failed
launch when a well-formed JSON-RPC error arrived on stdout.

`authenticate` with `gemini-api-key` and no `GEMINI_API_KEY` in the launch
environment returns `-32602` and does not authenticate.

## Persistence

`<home>` is `$GEMINI_HOME` when set and non-empty, otherwise `~/.gemini`.
Startup logged `Gemini home resolved to /Users/henry/.gemini (default;
$GEMINI_HOME is unset)`; setting `GEMINI_HOME=/tmp/gemhome` moved both the
resolved home and the settings path. Nothing is written to the config root
before authentication succeeds.

| Path | Contents |
| --- | --- |
| `<home>/antigravity-acp/settings.json` | `auth.type`, `gcp.project`, `gcp.location` |
| `<home>/antigravity-acp/acp_token.json` | `oauth-personal` token |
| `<home>/antigravity-acp/acp_business_token.json` | `oauth-business` token |
| `<home>/antigravity-acp/trusted_workspaces.json` | trusted workspace roots |
| `<home>/antigravity-acp/brain/` | harness per-conversation artifacts |
| `<home>/antigravity-acp/conversations/` | session trajectories |

Session lookup is scoped to the resolved home: an unknown identifier reports
`Session not found in the current GEMINI_HOME`. Resume therefore depends on the
config root being identical across relaunches, which is what makes per-task
`GEMINI_HOME` isolation incompatible with resume.

**Declared template versus resolved root.** Kandev declares one static
`SessionDirTemplate`, `{home}/.gemini/antigravity-acp`. That string is not
expanded against a home directory at launch. Its single production consumer is
the container bind-mount source, where `SessionDirHostPath` strips the `{home}/`
prefix and rebases the remainder onto `<kandev-home>/agent-sessions/<instance>/`
— a kandev-managed directory deliberately outside every home, so host state DBs
and session caches stay out of the container. That mount is created only when the
agent also declares a `SessionDirTarget`, and per
AC-AGENTS-ANTIGRAVITY-ACP-004.10 this agent declares none. For this agent the
template is therefore a *statement about where the server keeps its config root*
with no runtime effect today, not a path Kandev resolves.

As a statement it is accurate exactly when `GEMINI_HOME` is unset or empty; it
diverges when the environment exports a non-empty one, because the runtime
declaration inherits the environment unmodified and so the server sees it. That
divergence is not repairable here: the field is a static string on every agent,
consumed by container and remote paths that resolve it against a target the
local environment does not describe, so deriving it from a local variable would
be wrong for exactly the executors that consume it. Kandev declares the default
root, never reads `GEMINI_HOME`, and the requirements name any non-default
`GEMINI_HOME` as an explicit exclusion rather than pretending the template still
describes it. Remote credential upload has the same shape: it resolves sources
against `os.UserHomeDir()`, so it too addresses the default root only.

Skill resolution appends `<cwd>/.agents/skills/` and `<cwd>/.gemini/skills` for
the project, and `<home>/config/skills` plus `<home>/antigravity-cli/skills`
globally. Kandev declares the first of each pair — `.agents/skills` and
`.gemini/config/skills` — matching the single project and user skill directory
its injection contract supports. The server searches both entries of each pair,
so taking the first is a determinism choice rather than a capability one; it
also keeps Kandev out of `antigravity-cli/`, the directory the `agy` CLI owns
and the requirements isolate. Because that global pair resolves against `<home>`,
the declared user skill directory carries the same `GEMINI_HOME` divergence as
the session template: under a non-default `GEMINI_HOME` Kandev would inject user
skills where the server does not look, which is why the requirements name it in
that exclusion alongside session-dir seeding and the remote-auth copy.

Note that `<home>/antigravity-acp` nests inside `~/.gemini`, which is already
the Gemini agent's session directory template. The two remain distinct values;
neither is aliased to the other.

## Security

Credentials are written and read by the ACP server alone. Kandev copies token
files for remote executors and sets environment variables, and otherwise does
not create, modify, or delete anything under the config root.

API keys cannot be supplied over the ACP wire: `gemini-api-key` reads
`GEMINI_API_KEY` and `agent-platform` reads `GOOGLE_API_KEY` or
`GOOGLE_CLOUD_PROJECT`/`GOOGLE_CLOUD_LOCATION` with Application Default
Credentials, all from the launch environment. Remote auth therefore needs both a
file-copy method and environment-variable methods.

Which token file exists depends on the method the user chose, so a file-copy
method must tolerate absent entries rather than requiring all of them.

## Observability

The server logs to stderr in absl format at INFO by default. The two lines worth
recognising are `Gemini home resolved to <path>` at startup, which reports the
effective config root, and `Localharness not found.`, which is the only signal
of the partial-install state described above.

`--debug` raises verbosity. Kandev does not pass it; a user debugging an install
runs the binary directly.

## Deferred work

The requirements' `## Out of scope` names these exclusions; this is what a
follow-up would have to decide.

**Provisioning the artifact.** A five-platform URL matrix, where the cache lives,
how integrity is verified given Google publishes no checksum beside the archive,
how a rotating build-stamped URL is refreshed, and whether a ~894 MB extraction
is per-host or shared. Until that exists no image ships the binary, so the
containerized, Kubernetes, SSH and Sprites executors report it not found.

**A variable config root.** Supporting a non-default `GEMINI_HOME` means making
the declared template environment-derived, which the static `SessionDirTemplate`
contract shared by every agent does not allow today. Setting it per task or
session would additionally isolate credentials, forcing a fresh login per task,
and would break the resume property REQ-AGENTS-ANTIGRAVITY-ACP-004 depends on.

**Multi-variable remote auth.** `agent-platform` also resolves via
`GOOGLE_CLOUD_PROJECT` plus `GOOGLE_CLOUD_LOCATION` with Application Default
Credentials. A remote-auth method carries a single `EnvVar`, so that combination
cannot be expressed and an ADC-only remote host has no auth path until the method
shape grows.

## Related decisions

None. This design records an external vendor contract and introduces no new
Kandev architecture boundary.
