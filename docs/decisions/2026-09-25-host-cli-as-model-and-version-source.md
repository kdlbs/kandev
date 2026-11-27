# ADR-2026-09-25-host-cli-as-model-and-version-source: Use the Installed Vendor CLI as a Model and Version Source

**Status:** accepted
**Date:** 2026-09-25
**Area:** backend, frontend, protocol

## Context

Kandev launches Claude and Codex through managed ACP bridge packages pinned
to reviewed versions. The profile model dropdown shows only the models the
bridge advertises in its capability probe. The Claude bridge advertises a
short alias list (`default`, `opus[1m]`, `sonnet`, `haiku`, and one full
identifier), so an operator cannot pin a specific newly released model, and
the editor has no free-text entry. Operators also cannot see the installed
vendor CLI version, even though Kandev already detects the CLI on `PATH` for
availability and login.

Three sources of model truth exist: the bridge, the vendor CLI installed on
the host, and the vendor API. Only the Codex CLI exposes a documented
programmatic listing (`codex app-server` `model/list`). The Claude Code CLI
does not, on the versions verified during design.

Kandev already installs and upgrades vendor CLIs through the agent install
action, whose script is an `npm install -g` of the vendor package. Nothing
about that flow needs to change; what was missing is that its result was never
re-read.

## Decision

Each agent type may declare one compiled host CLI specification. Kandev reads
that CLI for its version and, when the vendor documents a listing, for its
model catalogue. The result is merged with the bridge list and labelled by
source. Agent types with a host CLI accept a custom model identifier typed by
the operator; every other agent type keeps the bridge-only list.

Discovery is read-only. Kandev adds no second mechanism for installing or
upgrading a vendor CLI: the existing agent install action stays the only one,
and discovery hangs off its completion hook so the version and model list
describe the binary now on disk. Discovery also runs when the backend finishes
loading agents, when the Agents settings page loads, and on the profile
editor's existing refresh.

The managed ACP bridge remains the session runtime. Kandev does not point the
bridge at the host CLI executable, and a custom model identifier is passed to
the bridge unchanged.

## Consequences

- New models appear in the Codex dropdown as soon as the installed CLI
  reports them, and any current identifier can be typed for Claude or Codex,
  without a Kandev release.
- Reinstalling an agent through the existing action immediately updates the
  advertised version and model list, with no backend restart.
- A custom identifier that the bridge cannot resolve fails at session start
  with the bridge's error; Kandev does not validate identifiers against the
  vendor API.
- The host CLI version and the bridge's bundled runtime can differ. Operators
  who need the bridge to run a newer Claude Code runtime still use the
  managed runtime update.
- Discovery costs one short subprocess per host CLI per freshness window.

## Alternatives Considered

- **Add a dedicated CLI update action.** Rejected: the agent install action
  already runs `npm install -g` for the vendor package, so a second action
  would duplicate an existing mechanism and split the maintenance interlock.
- **Point the bridge at the host CLI** (`CLAUDE_CODE_EXECUTABLE`,
  `CODEX_PATH`). This would make the host CLI the single source for sessions,
  models, and upgrades, but it replaces a reviewed, pinned runtime with an
  operator-managed binary of arbitrary version and breaks the validated
  managed runtime contract. Rejected for this iteration; it can be revisited
  as an explicit opt-in.
- **Query the vendor API for models.** Requires API credentials that
  subscription users do not have and would not reflect what the local CLI
  accepts. Rejected.
- **Keep a static list in Kandev and bump it per release.** This is the
  situation the change removes. Rejected.
- **Free-text only, no discovery.** Simpler, but the Codex CLI has a
  documented listing and the operator should not have to know identifiers by
  heart. Rejected.
