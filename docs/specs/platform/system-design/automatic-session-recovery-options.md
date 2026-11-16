---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
---

# Automatic session recovery options

See [session recovery convergence](session-recovery-convergence.md) for shared
ownership and recovery admission.

The runtime registry exposes three independent, restart-required options. All
shipped profiles default them off; an installation can enable them through the
existing runtime-settings API and Feature Toggles page or explicit environment.

- `features.autoStartPendingWork` starts a primary CREATED session only when its
  original initial submission is still pending, the task is already WIP-admitted
  with Autopilot enabled, its current step requests automatic start, dependencies
  are clear, and the session's existing Auto-run policy permits it. It reuses the
  original content and attachment descriptors, not a synthesized instruction.
- `features.autoContinueFromHistory` restores an eligible primary conversation
  after classified `native_state_missing` or `native_resume_unsupported`. It uses
  native Resume first. For Codex, this option also permits the provider's
  `thread/unarchive` only after `thread/read` returns the exact original ID and
  an archived rollout path, followed by same-ID Resume. No rollout file is
  searched, moved or changed by Kandev. A missing lookup is not deletion proof.
  Codex ACP instead negotiates its AIR `sessionIndex`/`sessionArchive` extension
  only when enabled. On a missing-session or internal-error load response, its
  original-cwd archived list must identify that exact ID with `archived=true`
  before `_session/unarchive {sessionId}` and same-ID load/resume. Windows
  extended DOS/UNC cwd notation is normalized without filesystem inspection.
  Authentication errors and failed/unsupported archive lookups preserve the
  original failure. This client adds no archive or delete request.
  If native Resume still reports a classified failure, it uses the existing
  history-only continuation and its generation/attempt settlement.
  An automatic attempt is admitted under the automatic ceiling, recorded once
  per recovery block or current session incarnation/generation, and never
  replays an uncertain continuation attempt. New bootstrap failures retain the
  bounded native reason and prompt-not-sent cause in the existing session error;
  an old generic failure is not retrospectively classified from prose.
- `features.autoRetryUnsentRequests` selects only durable queue claims whose
  existing SQL submission is `prepared` or `accepted`, has not crossed dispatch
  admission, and belongs to the current session incarnation/generation.
  SQL state alone is insufficient: the existing authenticated native inspection
  must identify that exact prepared/accepted submission and its terminated owner,
  with no live execution. A native dispatching record is never selected even
  when SQL still says accepted.
  It restores the same queue entry, submission identity and ordinary Auto-run
  delivery path. `dispatching`, interrupted/unknown, terminal, legacy, missing
  identity and uncertain initial-submission states remain parked.

The existing restart recovery pass owns these options; no additional timer,
daemon, workflow step, task, approval protocol or integrity check is introduced.
Archived/cancelled work, Office and automation ownership, explicit pauses,
non-admitted WIP, dependencies, permissions, model order and unresolved delivery
remain authoritative. Successful history restoration is not business completion.
Autopilot gates a new prepared task start, not recovery of an already accepted
primary conversation: controller tasks can legitimately have Autopilot disabled.
Any other STARTING/RUNNING session or live runtime of the same task leaves the
automatic primary recovery parked; a failed primary alone is not sole ownership.
Conversation recovery and pre-dispatch restoration retain their existing
Auto-run/explicit pause and automatic admission boundaries without changing that
task-creation setting.
These options do not turn a paused session's Auto-run policy on.
