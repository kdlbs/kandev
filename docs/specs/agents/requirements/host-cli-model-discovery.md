---
status: active
system: agents
created: 2026-09-25
owners:
  - kandev
---

# Host CLI Model Discovery Requirements

## Overview

Kandev launches Claude and Codex through managed ACP bridge packages. The
model dropdown in an agent profile shows only the models that the bridge
advertises during its capability probe. When a vendor releases a new model,
the operator cannot select it until the bridge advertises it, and the profile
editor offers no way to enter a model identifier by hand. Operators also
cannot see which vendor CLI version is installed on the Kandev host, so they
cannot tell whether a missing model means an old CLI.

The agent system owns provider model options and agent availability, so it
owns this capability. It adds the installed vendor CLI as a model and version
source for the agent types that have one, and keeps the bridge-advertised list
as the fallback for every other agent type.

Installing and updating a vendor CLI is already Kandev's agent install action;
this capability does not add a second mechanism. It keeps discovery honest
around that action instead: the model list and version must reflect the CLI
currently on disk.

## Terminology

- **Host CLI:** The vendor command-line tool installed on the Kandev host for
  an agent type, for example `claude` for Claude Code and `codex` for the
  Codex CLI. It is distinct from the managed ACP bridge package that Kandev
  launches for sessions.
- **Bridge list:** The model list advertised by the managed ACP bridge during
  the existing capability probe.
- **CLI list:** The model list obtained from the host CLI through a
  vendor-documented programmatic interface.
- **Custom model ID:** A model identifier the operator types instead of
  selecting from a list.
- **Agent install action:** The existing Kandev action that runs an agent
  type's install script, which installs or upgrades the vendor CLI and its
  bridge package.

## Requirements

### REQ-AGENTS-HOST-CLI-001: Host CLI version visibility

**Intent:** Operators can see which vendor CLI version Kandev detected so they
can reason about missing models and decide whether to reinstall.

**User story:** As an operator, I want the Agents settings page to show the
installed Claude Code and Codex CLI versions, so that I know which release
Kandev is working with.

#### Acceptance criteria

- **AC-AGENTS-HOST-CLI-001.1:** When an agent type declares a host CLI and the
  executable is detected, the system shall show the detected version next to
  the detected path on the agent card in Settings > Agents.
- **AC-AGENTS-HOST-CLI-001.2:** When the version command fails, times out, or
  prints no version, the system shall show that the version is unknown with a
  short reason and shall keep the agent available.
- **AC-AGENTS-HOST-CLI-001.3:** When the operator selects **Rescan**, or the
  agent install action completes successfully, the system shall re-detect the
  version without a backend restart.
- **AC-AGENTS-HOST-CLI-001.4:** Detecting a version shall not start an agent
  session, shall not require authentication, and shall complete within a
  bounded timeout.

### REQ-AGENTS-HOST-CLI-002: Dynamic model discovery from the host CLI

**Intent:** The model dropdown offers the models that the installed vendor CLI
knows about, so a newly released model appears without a Kandev code change,
and the operator can still enter any current model identifier when no
programmatic list exists.

**User story:** As an operator, I want the Claude and Codex profile model
dropdowns to reflect the installed CLI, so that I can select a model that was
released after the Kandev build.

#### Acceptance criteria

- **AC-AGENTS-HOST-CLI-002.1:** When an agent type declares a documented
  programmatic model listing on its host CLI, the system shall query that
  listing, shall merge the result with the bridge list, and shall label each
  model with its source in the profile model selector.
- **AC-AGENTS-HOST-CLI-002.2:** When an agent type declares no programmatic
  model listing, the system shall present the bridge list with a visible note
  that the vendor CLI does not publish a model list, and shall offer a custom
  model ID entry.
- **AC-AGENTS-HOST-CLI-002.3:** When the operator types a model identifier that
  is not in the list for an agent type that allows custom model IDs, the
  selector shall offer that exact text as a selectable entry, and saving the
  profile shall persist it unchanged.
- **AC-AGENTS-HOST-CLI-002.4:** A discovered list shall be reused for later
  requests until a refresh, a successful agent install, or the discovery
  freshness window invalidates it. Listing models shall not start a vendor
  process on the request path.
- **AC-AGENTS-HOST-CLI-002.5:** When the host CLI is not installed, is not
  logged in, exits with an error, or exceeds the discovery timeout, the system
  shall keep the bridge list, shall show a localized reason with the retry
  action, and shall not block the profile editor.
- **AC-AGENTS-HOST-CLI-002.6:** Agent types other than Claude and Codex shall
  keep their current bridge-only behavior with no visible change.

### REQ-AGENTS-HOST-CLI-003: Discovery refresh around agent load and install

**Intent:** The version and model list always describe the CLI currently on
disk, without adding a second install or update mechanism and without
requiring a backend restart.

**User story:** As an operator, I want Kandev to re-read a vendor CLI after it
loads an agent and after I install or upgrade that agent, so that the model
dropdown matches the CLI I am running.

#### Acceptance criteria

- **AC-AGENTS-HOST-CLI-003.1:** When the backend finishes loading agents, the
  system shall discover versions and model lists for each detected host CLI
  and populate its caches, without delaying startup.
- **AC-AGENTS-HOST-CLI-003.2:** When the Agents settings page loads or the
  operator selects **Rescan**, the system shall re-detect versions and shall
  re-read any model catalogue outside its freshness window, without delaying
  the page response.
- **AC-AGENTS-HOST-CLI-003.3:** When the existing agent install action
  completes successfully for an agent type with a host CLI, the system shall
  discard that agent's cached version and model catalogue and shall rediscover
  both, so later reads describe the newly installed CLI without a backend
  restart.
- **AC-AGENTS-HOST-CLI-003.4:** The operator shall be able to force a fresh
  read from the profile editor's existing capability refresh action.
- **AC-AGENTS-HOST-CLI-003.5:** The system shall not add a second install or
  update path for a vendor CLI. The existing agent install action remains the
  only mechanism that changes an installed CLI.

### REQ-AGENTS-HOST-CLI-004: Stored model compatibility

**Intent:** Existing profiles keep working when their stored model is absent
from the discovered lists.

#### Acceptance criteria

- **AC-AGENTS-HOST-CLI-004.1:** When a saved profile references a model that is
  absent from both lists for an agent type that allows custom model IDs, the
  selector shall show it as a selectable custom entry with its identifier, and
  saving the profile without changing the model shall persist it unchanged.
- **AC-AGENTS-HOST-CLI-004.2:** When a saved profile references an absent model
  for an agent type that does not allow custom model IDs, the selector shall
  keep the existing unavailable presentation.
- **AC-AGENTS-HOST-CLI-004.3:** A failed or skipped CLI discovery shall never
  rewrite a saved profile model.

## Out of scope

- A second install or update mechanism for a vendor CLI. The existing agent
  install action already reinstalls the CLI at its latest published version.
- Changing which executable the managed ACP bridge uses for sessions. Session
  launches continue to use the trusted managed runtime package described in
  [Managed Agent Runtime Versions and Updates](runtime-updates.md).
- Model discovery for agent types other than Claude and Codex.
- Validating a custom model ID against the vendor API before a session starts.
- Reading a vendor CLI inside containers, SSH hosts, or Kubernetes executors.
