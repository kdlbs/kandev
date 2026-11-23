---
created: 2026-11-22
status: in_progress
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-002
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
legacy_specs: []
---

# Repair PostgreSQL built-in prompt refresh

The desktop PostgreSQL upgrade rehearsal exits before readiness when it replays
the utility store schema with a shipped legacy Enhance prompt. The
[run before the viewer merge](https://github.com/kdlbs/kandev/actions/runs/45032559541)
and the [latest examined run](https://github.com/kdlbs/kandev/actions/runs/45042343697)
have the same candidate startup error. The SQLite desktop and mobile scenarios
pass in the latter run.

The refresh reads the integer flag into an integer, then sends a Go boolean
as the value for that same column in its conditional update. PostgreSQL rejects
that value. The repair must preserve the existing guarded refresh and startup
failure handling.

The [utility requirement](../../specs/agents/requirements/utility-agent-profiles.md)
and [utility design](../../specs/agents/system-design/utility-agent-profiles.md)
already describe the intended behavior. The [upgrade requirement](../../specs/platform/requirements/release-upgrade-regression.md)
requires independent results on SQLite and PostgreSQL. The Enhance utility
refresh is eligible by known prompt hash and conditionally updates the original
prompt text. Saved-prompt refreshes retain both known-hash and timestamp
eligibility checks.

1. [Repair and verify the prompt refresh](task-01-repair-prompt-refresh.md).
2. Add the utility store to the fixed PostgreSQL 16 CI cohort and verify that
   the workflow contract test confirms the package runs with its job-level DSN.
3. Confirm the fixed source passes both upgrade engines using the existing
   release rehearsal. Keep results separate by engine and viewport.
4. Track remaining viewer validation separately in the
   [viewer host work order](../file-viewer-plugins/task-02-viewer-api.md) and
   [release validation record](../../reviews/file-viewers-implementation.md).
   The remote runs need a Docker-enabled runner and viewer-specific scenarios
   for Docker, SSH, and Kubernetes. Repeated previews under memory pressure need
   a physical phone. Record OS, browser, model, and result for that run.

Release and publication remain on hold.
