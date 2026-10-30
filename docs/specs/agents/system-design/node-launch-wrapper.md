---
status: current
system: agents
created: 2026-10-30
requirements:
  - REQ-AGENTS-NODEWRAP-001
  - REQ-AGENTS-NODEWRAP-002
  - REQ-AGENTS-NODEWRAP-003
  - REQ-AGENTS-NODEWRAP-004
  - REQ-AGENTS-NODEWRAP-005
---

# Node launch wrapper system design

## Purpose and ownership

Agents owns launch semantics across lifecycle, host utility, and agentctl.
This design adds one operator-owned wrapper that agentctl applies to every agent launch.
It reuses the `launchwrapper` package, its tokens, and its preparation steps.
It adds no provider registry entry, plugin API, or profile field.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-NODEWRAP-001 | Configuration, Validation |
| REQ-AGENTS-NODEWRAP-002 | Composition |
| REQ-AGENTS-NODEWRAP-003 | Launch coverage, Failure modes |
| REQ-AGENTS-NODEWRAP-004 | Identity |
| REQ-AGENTS-NODEWRAP-005 | Launch coverage, Backend passthrough runner |

## Configuration

agentctl takes two flags:

| Flag | Meaning |
| --- | --- |
| `--node-launch-wrapper-file PATH` | File holding the node wrapper envelope |
| `--require-node-launch-wrapper` | Refuse startup and every launch unless a wrapper is loaded |
| `--node-launch-wrapper-owner-uid UID` | Expected owner. Default `-1` means the agentctl process owner |

The file holds the existing envelope with source `node`:

```json
{"version": 1, "source": "node", "argv": ["/opt/wrap/launch", "--root", "{{kandev.work_dir}}", "--"]}
```

Tokens follow the generic grammar: whole-argument `{{kandev.work_dir}}` and `{{kandev.managed_npm_dir}}`, triple-brace escape, no reference in the executable position.
The executable shall be an absolute path, because there is no working directory or profile PATH at startup to resolve against.
Decoding is strict. Unknown fields and any source other than `node` are refused.
`Spec.Validate` is unchanged and accepts only `profile`, so no wire request that feeds a profile path can carry source `node`.
A separate loader returns the `node` envelope.
Configuration is read once at startup and held in `config.Config`, then copied into each instance configuration with `json:"-"`, so it is never serialized, snapshotted, or accepted from a request.

## Validation

The loader runs before agentctl serves any request and refuses on the first failure with a message naming the stage.
It never falls back to an unwrapped launch.

1. The path is resolved with symlinks evaluated. The checks below apply to the resolved path.
2. The file is opened once. Owner and mode come from `fstat` on that descriptor, and the content is read from it, so the checked file is the read file.
3. The file shall be a regular file owned by the owner, with no group or world write bit.
4. Every directory above the file shall be owned by the owner or root and shall not be group- or world-writable. No sticky exception exists: a sticky bit does not excuse a group- or world-writable directory. Production wrapper paths such as `/opt/...` are never sticky, so the system temporary directory is not a valid location.
5. The executable at `argv[0]` shall be a regular executable file and meet rules 3 and 4 after symlink resolution.
6. The envelope shall decode strictly and pass token validation.

The expected owner is configuration. The default is the effective user of agentctl. Root is accepted only as an ancestor owner.
Windows has no equivalent mode model here, so a node wrapper file is refused on Windows.
The check is a startup check. It does not defend against an operator, or a process running as the owner, who edits the files afterwards.

## Composition

`launchwrapper.PrepareNode(node, profile, inner, ctx)` builds the final argv in one place:

1. If a profile wrapper exists, prepare it around the inner command exactly as today, including managed npm prefix handling.
2. Prepare the node wrapper around that result, without repeating npm handling.
3. If no node wrapper is configured, the function is the existing `Prepare`, so output is identical.

Worked example. The inner command is `agent --acp`. The profile wrapper is `["prof", "--"]`. The node wrapper is `["/opt/wrap/launch", "--root", "{{kandev.work_dir}}", "--"]`, with work directory `/w`:

```text
/opt/wrap/launch --root /w -- prof -- agent --acp
```

The node argv is the first argv of the command, a profile can only add arguments after it, and clearing the profile yields `/opt/wrap/launch --root /w -- agent --acp`.
The node wrapper is held outside the profile wrapper fields, which `configure` rebuilds from request input, so a configure request cannot clear it.
The env passed to the wrapper is the same final child environment as for the profile wrapper.

## Launch coverage

Every place agentctl starts an agent goes through `PrepareNode`.

| Seam | Where | Treatment |
| --- | --- | --- |
| Managed agent process | `process.Manager.buildFinalCommand` through `prepareSessionArgs` | Wrapped |
| One-shot initial and continue args | `prepareOneShotArgs`, prepared from a retained pristine copy so restarts never wrap twice | Wrapped |
| Interactive passthrough (agentctl) | `InteractiveRunner.startProcess` | Wrapped |
| ACP utility prompt | `ACPInferenceExecutor.Execute` through `prepareACPLaunch` | Wrapped |
| ACP probe | `ACPInferenceExecutor.Probe` through `prepareACPLaunch` | Wrapped |
| OpenCode secondary model listing | `probeOpenCodeModels` through `prepareACPLaunch` | Wrapped |
| Codex app-server utility | `CodexAppServerInferenceExecutor.start` | Wrapped |
| Runtime observation | `collectRuntimeObservation` | Skipped while a node wrapper is active, as for a profile wrapper, so no agent-adjacent executable runs unwrapped |

Not agent launches, and not covered: scripts and user processes (`ProcessRunner.Start`, `StartPiped`), user shells, ACP client terminals, language servers, code-server, git and GitHub helpers, MCP server processes.
The kandev backend's own `InteractiveRunner` is covered by a fail-closed rule, described next.

A launch seam is refused, not left unwrapped, when a wrapper is required and not loaded.

## Backend passthrough runner

The standalone executor's `InteractiveRunner` runs in the backend process and cannot read agentctl flags or apply a node wrapper.
The backend has no configuration for a node wrapper. Its only signal is the capability set of the agentctl it adopted at startup: `node-launch-wrapper.v1` is present only while that agentctl has a wrapper loaded.
When the capability is present, `backendapp` marks the runner with `SetNodeLaunchWrapper(Node{Required: true})`. `InteractiveRunner.Start` then refuses any non-user-shell request that carries no wrapper of its own, with an error that names the missing capability and starts no process. All passthrough starts, including restart and resume, go through `Start`.
This is a fail-closed rule, not coverage: the runner never applies a wrapper.
Gap: when the backend has no adopted agentctl, for example it launched its own child without the node flags, or the node wrapper was added after adoption, no signal exists and the runner behaves as before. No new host contract is introduced for that case.

## Failure modes

| Condition | Result |
| --- | --- |
| Required, no file | agentctl refuses to start |
| File missing, unreadable, not owned, writable, bad JSON, wrong source, bad token | agentctl refuses to start |
| Required, wrapper absent from configuration at launch | Launch refused, no child |
| Wrapper executable missing or not executable at launch, work directory missing | Launch refused with the existing generic classification, no child |
| Backend passthrough runner, peer advertises the capability | Launch refused, no child |
| Wrapper fails or exits non-zero | Treated as a process failure with the existing sanitized messages, no unwrapped retry |

Messages and logs name the stage and never include the wrapper argv or wrapper stderr.
Where a profile wrapper already redacts the launch (`[wrapped agent]`, "wrapped utility process failed to start"), an active node wrapper does the same.

## Identity

`GET /identity` on the control server adds the capability `node-launch-wrapper.v1` when a node wrapper is loaded, and omits it otherwise.
The route returns no path, argument, or content.
A host or plugin that needs a confinement guarantee verifies the capability and then verifies the confinement itself.
The capability proves application of the wrapper only.
