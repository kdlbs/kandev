# ADR-2026-07-25-scheduled-core-agent-version-pins: Scheduled Core Agent Version Pins

**Status:** accepted
**Date:** 2026-07-25
**Area:** backend, infra, workflow

## Context

Kandev launches several core ACP agents from npm packages. Claude ACP, Codex
ACP, and OpenCode already use exact versions, while Copilot and Gemini resolve
the npm `latest` tag on each `npx` invocation. Their operational versions are
repeated across Go constants, contract tests, and current-version
documentation, so generic dependency bots cannot update them safely.

Cursor uses its official installer instead of npm. The supported Cursor CLI
channel installs the current build and auto-updates by default, without a
supported immutable version selector or CLI auto-update opt-out.

## Decision

Kandev maintains exact runtime package versions for the five core
npm-distributed agents: Claude ACP, Codex ACP, OpenCode, Copilot, and Gemini.
A repository-owned updater queries each package's stable npm `latest` dist-tag,
updates only an explicit allowlist of current-version files, and fails when the
expected source shape or replacement count drifts.

A weekly GitHub Actions workflow runs the updater and opens or refreshes one
grouped pull request when any core pin changes. The pull request is never
auto-merged. Candidate packages are not executed in the job that holds the
GitHub App write token; compatibility evidence comes from repository tests,
normal pull-request CI, and maintainer review.

Cursor remains intentionally unpinned until its supported distribution channel
offers an enforceable version selector and a way to prevent runtime
auto-updates. The updater does not infer private Cursor artifact URLs from the
mutable installer.

The OpenCode binary used by the trusted PR-review workflow remains outside this
automation. Its release asset and checksum form a separate trust-sensitive pin.
Other npm-backed agents remain on their existing latest-channel behavior until
they are deliberately added to the core set.

Historical changelogs and ADR version examples are not updater targets.
Current-version documentation points to the maintained pin inventory instead.

## Consequences

Core agent launches become reproducible, and version drift is surfaced in a
reviewable weekly pull request. Copilot and Gemini no longer pick up upstream
changes between Kandev releases without repository review.

The allowlist and replacement-count checks require intentional maintenance when
pin locations move. Grouped updates reduce pull-request noise but require
independent per-agent validation in the pull-request report.

Cursor can still change outside a Kandev release because its official client
owns updates. Kandev documents that limitation instead of claiming a pin it
cannot enforce.

## Alternatives Considered

- Dependabot or Renovate custom regex managers were rejected because the pins
  span Go constants, tests, and prose, and because exact replacement counts are
  part of the drift-safety contract.
- Pinning every npm-backed Kandev agent was rejected to keep the first policy
  focused on the user-selected core set.
- Downloading Cursor from a version-shaped URL extracted from its mutable
  installer was rejected because it is not a supported immutable channel and
  would not prevent the installed CLI from auto-updating.
- Using `GITHUB_TOKEN` alone was rejected because a narrowly permissioned
  GitHub App token lets the automated pull request trigger normal CI without a
  maintainer approval step.
