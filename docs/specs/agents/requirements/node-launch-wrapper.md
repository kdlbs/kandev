---
status: active
system: agents
created: 2026-10-30
owners:
  - kandev
---

# Node launch wrapper requirements

## Overview

A node operator can require every agent process started by one agentctl to run through one operator-owned wrapper.
The wrapper is a generic launch prefix. Core gives it no product, sandbox, or provider meaning.
A sandbox is one possible configuration of it, supplied and verified outside core.
A wrapper's presence alone does not prove confinement.

This contract extends [Generic launch wrappers](generic-launch-wrappers.md).
It narrows that contract's exclusion of an executor-level wrapper setting to node-level operator wrappers and changes nothing else in it.
The design is in the [node launch wrapper system design](../system-design/node-launch-wrapper.md).

## Terms

- **Node wrapper:** A wrapper that agentctl reads at startup from an operator-owned file. Its source value is `node`.
- **Profile wrapper:** A wrapper from profile launch settings, as defined by the generic contract. Its source value is `profile`.
- **Agent launch:** Any process agentctl starts to run an agent, or to run an agent for a probe or utility inference.
- **Allocation directory:** A directory the node's scheduler client creates for one workload, held outside the agent's writable workspace. The node operator hands it to agentctl at startup. A node wrapper can reference it to keep its own state where the agent cannot rewrite it. Core gives it no scheduler meaning.
- **Owner:** The user id that must own the wrapper file, the wrapper executable, and the directories above them. It defaults to the agentctl process owner. Root is also accepted as an owner of ancestor directories.

## Requirements

### REQ-AGENTS-NODEWRAP-001: Operator-owned configuration

**Intent:** Only the node operator, never a profile, task, or agent, can set the node wrapper.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-001.1:** agentctl shall read the node wrapper only from the file named by `--node-launch-wrapper-file`. No API request, profile field, task input, or agent environment shall set, replace, or clear it.
- **AC-AGENTS-NODEWRAP-001.2:** The file shall hold one wrapper envelope with version 1 and source `node`. An envelope with another source, unknown fields, an unknown or misplaced runtime reference, an empty or flag-like executable, or a non-absolute executable shall be refused. Source `profile` shall keep its present behavior and shall not accept source `node`.
- **AC-AGENTS-NODEWRAP-001.3:** agentctl shall refuse to start when the file, the wrapper executable, or any directory above either is not owned by the owner or root, or is group- or world-writable. A sticky bit grants no exception. The owner shall be configurable through `--node-launch-wrapper-owner-uid` and shall not be fixed to an account.
- **AC-AGENTS-NODEWRAP-001.4:** `--require-node-launch-wrapper` without a file shall refuse startup. A named file that is missing or invalid shall refuse startup whether or not it is required.

### REQ-AGENTS-NODEWRAP-002: Outermost composition

**Intent:** A profile wrapper cannot remove, reorder, or bypass the node wrapper.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-002.1:** When a node wrapper is active, the first argument vector of every agent launch shall be the node wrapper. A profile wrapper, when present, shall follow it, and the agent command shall follow both.
- **AC-AGENTS-NODEWRAP-002.2:** Profile configuration, including configuration that clears or omits the profile wrapper, shall not change or remove the node wrapper. The node wrapper shall not appear in the profile wrapper snapshot.
- **AC-AGENTS-NODEWRAP-002.3:** Node wrapper runtime references shall resolve as defined by AC-AGENTS-WRAPPERS-002.1 through 002.3.

### REQ-AGENTS-NODEWRAP-003: Coverage and refusal

**Intent:** An active node wrapper is never skipped by an agent launch path.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-003.1:** Initial sessions, one-shot initial and continuation commands, interactive passthrough, ACP utility prompts, ACP probes, secondary model discovery, and the Codex app-server utility shall each start through the node wrapper exactly once.
- **AC-AGENTS-NODEWRAP-003.2:** Runtime observation that would run an agent-adjacent executable directly shall not run while a node wrapper is active.
- **AC-AGENTS-NODEWRAP-003.3:** When a node wrapper is required and not loaded, or cannot be prepared at launch, the launch shall be refused with no child process and no unwrapped retry, as in AC-AGENTS-WRAPPERS-004.1.
- **AC-AGENTS-NODEWRAP-003.4:** When no node wrapper is configured, behavior shall be unchanged.
- **AC-AGENTS-NODEWRAP-003.5:** Failure responses and logs shall not expose wrapper arguments or unfiltered wrapper stderr, as in AC-AGENTS-WRAPPERS-004.5.

### REQ-AGENTS-NODEWRAP-004: Verifiable identity

**Intent:** A host or plugin can check that a node wrapper is active without learning its contents.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-004.1:** agentctl shall advertise the capability `node-launch-wrapper.v1` on its identity route only while a node wrapper is loaded.
- **AC-AGENTS-NODEWRAP-004.2:** No route shall return the wrapper path, arguments, or file contents.
- **AC-AGENTS-NODEWRAP-004.3:** The capability shall mean only that the wrapper is applied. It shall not claim confinement, as in AC-AGENTS-WRAPPERS-005.3.

### REQ-AGENTS-NODEWRAP-005: Backend passthrough fails closed

**Intent:** An agent launch that cannot be wrapped never runs bare on a node that requires wrapping.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-005.1:** The kandev backend's own passthrough runner runs in the backend process and cannot apply an agentctl node wrapper. Its use is outside the wrapper contract for that process.
- **AC-AGENTS-NODEWRAP-005.2:** When the agentctl the backend adopted advertises `node-launch-wrapper.v1`, the backend's passthrough runner shall refuse every agent launch with an error that says it cannot apply the wrapper, and shall start no process. User shells are not agent launches.
- **AC-AGENTS-NODEWRAP-005.3:** When that capability is absent, the runner shall behave as before.
- **AC-AGENTS-NODEWRAP-005.4:** The capability is the only signal the backend has. A node wrapper that the backend did not learn about through an adopted agentctl is not detected, and this contract does not claim to cover it.

### REQ-AGENTS-NODEWRAP-006: Allocation directory reference

**Intent:** A node wrapper can name a per-workload directory outside the workspace, without core knowing which scheduler provides it.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-006.1:** A node wrapper argument that is exactly `{{kandev.alloc_dir}}` shall be accepted after the executable. It shall be refused as the executable and in any partial, embedded or malformed form. A profile wrapper shall not accept it, and `{{{kandev.alloc_dir}}}` shall stay the literal text, as in AC-AGENTS-WRAPPERS-002.3.
- **AC-AGENTS-NODEWRAP-006.2:** The reference shall resolve to the value of `--node-launch-wrapper-alloc-dir` after symlinks are resolved. The value shall be an absolute path to an existing directory owned by the agentctl process owner and not writable by group or others, and it shall not be inside the workspace of the launch.
- **AC-AGENTS-NODEWRAP-006.3:** agentctl shall resolve and check the value again at every launch that uses the reference. No process environment variable, API request, profile field, task input, or agent environment shall supply or replace it.
- **AC-AGENTS-NODEWRAP-006.4:** A node wrapper that references the allocation directory shall refuse startup when the value is missing or fails the checks in AC-AGENTS-NODEWRAP-006.2. A launch that finds the value unusable shall be refused with no child process and no unwrapped retry. A node wrapper that does not reference it shall behave as before whether or not a value is supplied.
- **AC-AGENTS-NODEWRAP-006.5:** The value shall be held only in agentctl's startup configuration, never serialized, snapshotted, or advertised. Failure messages shall name the failed check and shall not include the path.
- **AC-AGENTS-NODEWRAP-006.6:** Supporting the reference shall not change the advertised capability `node-launch-wrapper.v1`. An agentctl that lacks the reference refuses both the flag and a policy that uses it, so the combination fails at startup and never runs unwrapped.

## Exclusions

No per-task or per-profile override, automatic permission grant, provider-name exception, sandbox plugin API, approval transport, or policy learning ships with this contract.
Processes that are not agent launches are outside it: scripts and user processes, user shells, language servers, code-server, ACP client terminals, and MCP server processes.
A deployment that must confine those paths needs a separate contract.
No new rendered controls or layout changes are required.
