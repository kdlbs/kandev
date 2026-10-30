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
- **Owner:** The user id that must own the wrapper file, the wrapper executable, and the directories above them. It defaults to the agentctl process owner. Root is also accepted as an owner of ancestor directories.

## Requirements

### REQ-AGENTS-NODEWRAP-001: Operator-owned configuration

**Intent:** Only the node operator, never a profile, task, or agent, can set the node wrapper.

#### Acceptance criteria

- **AC-AGENTS-NODEWRAP-001.1:** agentctl shall read the node wrapper only from the file named by `--node-launch-wrapper-file`. No API request, profile field, task input, or agent environment shall set, replace, or clear it.
- **AC-AGENTS-NODEWRAP-001.2:** The file shall hold one wrapper envelope with version 1 and source `node`. An envelope with another source, unknown fields, an unknown or misplaced runtime reference, an empty or flag-like executable, or a non-absolute executable shall be refused. Source `profile` shall keep its present behavior and shall not accept source `node`.
- **AC-AGENTS-NODEWRAP-001.3:** agentctl shall refuse to start when the file, the wrapper executable, or any directory above either is not owned by the owner or root, or is group- or world-writable. A sticky directory owned by root is accepted. The owner shall be configurable through `--node-launch-wrapper-owner-uid` and shall not be fixed to an account.
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

## Exclusions

No per-task or per-profile override, automatic permission grant, provider-name exception, sandbox plugin API, approval transport, or policy learning ships with this contract.
Processes that are not agent launches are outside it: scripts and user processes, user shells, language servers, code-server, ACP client terminals, and MCP server processes.
The kandev backend's own passthrough runner runs outside agentctl and is outside it.
A deployment that must confine those paths needs a separate contract.
No new rendered controls or layout changes are required.
