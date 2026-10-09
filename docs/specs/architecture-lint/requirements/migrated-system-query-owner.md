---
status: active
system: architecture-lint
created: 2026-10-08
owners:
  - kandev
---

# Migrated System Query Ownership Guard Requirements

## Overview

This internal repository-tooling contract protects four accepted frontend
ownership boundaries after their read snapshots moved to TanStack Query. It
helps developers identify a supported Zustand mirror during ordinary frontend
verification. It does not add or change product behavior.

## Terminology

- **Migrated snapshot:** One of the About SystemInfo, database-statistics,
  backup-list, or disk-usage Query resources listed in the system design.
- **Supported mirror:** A statically recognizable field, default, action,
  compatibility type/export, or explicit hydration write at the agreed System
  Zustand owner boundaries.

## Requirements

### REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001: Protect migrated System Query ownership

**Intent:** Keep a second Zustand owner from silently returning for a migrated
System snapshot while preserving unrelated System state and legitimate Query
usage.

#### Acceptance criteria

- **AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.1:** When a supported
  mirror for a migrated snapshot is present at a guarded System owner boundary,
  frontend lint shall fail with the source location, resource name, and
  supported Query hook that owns the snapshot.
- **AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.2:** When a guarded
  file contains an allowed System field, legitimate Query usage, an unrelated
  domain shape, or a snapshot name only in a comment, string, or template,
  frontend lint shall not report a migrated-owner finding.
- **AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.3:** The normal web
  lint path shall execute the guard on the current guarded production files and
  complete without a migrated-owner finding.
- **AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.4:** The web test
  suite shall verify the real ESLint configuration enables the guard for each
  agreed owner boundary and detects supported mirrors there. These tests shall
  fail if the rule is removed, disabled, or omitted from any owner boundary.

## Out of scope

- Changing Query behavior, HTTP requests, cache freshness, cancellation,
  providers, boot payloads, UI behavior, or backend contracts.
- Proving arbitrary aliases or data flow, or prohibiting other Zustand state,
  Query calls, API types, or snapshot-related words.
- Expanding the Python architecture-lint rule registry or its baselines.
