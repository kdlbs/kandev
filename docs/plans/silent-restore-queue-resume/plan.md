---
created: 2026-10-20
status: done
requirements:
  - REQ-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001
system_design:
  - ../../specs/platform/system-design/silent-restore-queue-resume.md
legacy_specs: []
---

# Implementation Plan: Opt-in queue resume after silent restart restore

## Overview

A successful silent restart restore leaves the Auto-run pause it set, so queued
messages stop after every backend restart. An opt-in flag returns Auto-run to
its earlier state once the restore succeeds.

## Scope

In scope: `features.resumeQueueAfterSilentRestore` (config, runtime flag
registry, profiles, orchestrator service config, public configuration table)
and recovery-owned queue pause/resume through the existing restore and queue
admission paths.

Excluded: any change with the flag off, dispatch while restore locks are held,
and the cancel-path pause. The opt-in completion uses the existing owned queue
drain after those locks release; it adds no scheduler or timer.

## Work orders

- [x] [Task 01: Resume the queue a successful silent restore paused](task-01-silent-restore-queue-resume.md)

## Changed-path coverage

The work order owns the orchestrator, config, runtime flag, profile and public
docs changes, focused verification, and the new requirement and design.
