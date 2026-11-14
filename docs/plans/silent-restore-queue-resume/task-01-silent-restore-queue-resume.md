---
id: "01-silent-restore-queue-resume"
title: "Resume the queue a successful silent restore paused"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001
acceptance_criteria:
  - AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.1
  - AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.2
  - AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.3
system_design:
  - ../../specs/platform/system-design/silent-restore-queue-resume.md
---

# Task 01: Resume the queue a successful silent restore paused

## Summary

Behind `features.resumeQueueAfterSilentRestore` (off in every profile), a
successful silent restore re-enables the Auto-run it paused, and only if it was
on before.

## Verification

The original focused queue checks are reused through a temporary Go overlay,
covering the default pause, opt-in resume, and preservation of a user pause.
The existing `TestRestartRecoveryRestoresNativeConversationWithoutPrompt`
checks restore behavior with the flag off. Registry/profile and frontend
feature-contract checks cover the default-off configuration wiring. No new
standing test file is part of this port.

The 2026-10-23 follow-up reproduced six failure cases against the published
implementation, then passed nine focused recovery/queue probes using real
SQLite state. The existing selected orchestrator and messagequeue tests passed.
The ordinary dispatch probe additionally waited for the existing execution
completion hook before fixture cleanup. See the design's validation section for
the tested policy, commit and drain boundaries. Temporary overlays were removed
after validation.
