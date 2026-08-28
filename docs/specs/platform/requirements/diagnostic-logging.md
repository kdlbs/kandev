---
status: active
system: platform
created: 2026-07-30
updated: 2026-08-27
owners:
  - tbd
---

# Diagnostic logging Requirements

## Overview

Users can see frontend failures that leave no backend evidence, and support cannot rely on one known artifact containing recent backend and browser diagnostics. The product needs a clearly disclosed bundle that users can download and agents can inspect without continuously uploading browser console history or returning an unbounded log export.

## Requirements

### REQ-PLATFORM-DIAGNOSTIC-LOGGING-001: Diagnostic logging

**Intent:** Users can see frontend failures that leave no backend evidence, and support cannot rely on one known artifact containing recent backend and browser diagnostics. The product needs a clearly disclosed bundle that users can download and agents can inspect without continuously uploading browser console history or returning an unbounded log export.

#### Acceptance criteria

- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.1:** Every backend process targets `<Kandev home>/logs/backend-logs.log`, where `<Kandev home>` is the resolved `KANDEV_HOME_DIR` or its `~/.kandev` default, and writes there whenever the path is available.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.2:** Backend startup prints the absolute log-file path to stdout before the backend becomes ready.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.3:** A normal run writes `info` and higher entries to the file and `warn` and higher entries to stdout.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.4:** A debug run writes `debug` and higher entries to the file while stdout remains `warn` and higher.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.5:** An explicitly verbose run writes `info` and higher entries to both the file and stdout.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.6:** `logging.level` / `KANDEV_LOG_LEVEL` remains the supported override for the file threshold, and `logging.format` remains supported. `logging.outputPath`, `logging.maxSizeMb`, `logging.maxBackups`, `logging.maxAgeDays`, and `logging.compress` are removed; the diagnostic path and daily retention policy are not configurable.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.7:** `backend-logs.log` represents the current UTC calendar day. At UTC midnight, the backend atomically rolls it to `backend-logs-YYYY-MM-DD.log` and creates a new owner-readable, owner-writable active file.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.8:** A restart during the same UTC day appends to `backend-logs.log`. On the first startup after a day boundary, the previous active file is rolled to the date it represents before new entries are written.
- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.9:** A browser debug diagnostic
  that scans a growing application collection shall compute and emit at most
  one latest-state sample per 250 ms while that collection changes
  continuously.

## System design

The migrated technical source is split into [part 1](../system-design/diagnostic-logging-01.md), [part 2](../system-design/diagnostic-logging-02.md).
