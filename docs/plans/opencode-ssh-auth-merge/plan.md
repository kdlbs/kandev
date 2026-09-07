---
created: 2026-09-05
status: done
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
legacy_specs: []
---

# Implementation Plan: Preserve OpenCode SSH Credentials

## Overview

The fix adds an agent-owned policy for credential-file conflicts. OpenCode uses that policy to merge provider maps before a remote write.

One vertical work order changes the descriptor, transfer layer, transports, tests, and public executor reference. This order keeps one regression boundary.

## Scope

### In scope

- Preserve OpenCode providers that exist only in the target `auth.json`.
- Copy source-only providers and replace same-provider target entries with source entries.
- Leave an unreadable or malformed existing target unchanged.
- Keep the existing replace policy for other credential files.
- Document the SSH credential-copy behavior.

### Out of scope

- A user-selectable conflict policy.
- Timestamped credential backups.
- A remote lock for concurrent OpenCode writes.
- Changes to environment-based credential methods.

## Technical approach

Add an internal existing-file policy to `agents.RemoteAuthMethod`. Copy the policy into `remoteauth.Method` without adding a frontend contract.

Set the JSON-object merge policy only on the OpenCode file method. Keep replacement as the zero-value policy.

Add a narrow target-read interface beside `FileUploader`. Implement it for the persistent Sprites and SSH transports.

Isolated local and Kubernetes targets validate and write the source object. They do not require a target read.

Update `UploadCredentialFiles` to read and merge only when the selected method declares the merge policy. Return an error before each write when the merge input is invalid.

Use deterministic JSON encoding and private file mode for merged output. Keep existing write behavior for a missing target and for all replacement methods.

Update the SSH executor reference in `docs/public/executors.md`. State the OpenCode merge and malformed-file behavior.

## Tests

- `AC-EXECUTORS-SSH-EXECUTOR-001.11`: Add transfer tests for target-only, source-only, and same-provider entries.
- `AC-EXECUTORS-SSH-EXECUTOR-001.11`: Add an SSH integration test with an existing remote OpenCode provider map.
- `AC-EXECUTORS-SSH-EXECUTOR-001.12`: Add transfer and SSH tests that preserve malformed target bytes.
- Add a transfer test that keeps an isolated OpenCode credential copy working.
- Add catalog tests that make sure only OpenCode declares the merge policy.
- Keep a replacement test for an opaque credential file.

## Work orders

- [x] [Task 01: Merge OpenCode credential provider maps](task-01-merge-opencode-credential-providers.md)

## Verification results

- Focused Go tests: 19 passed in three packages.
- Specification linter: passed.
- Public documentation tests: 61 passed.
- Published documentation validator: 45 pages passed.
- Git diff check: passed.

## Risks

- A source-wins collision replaces the remote login for the same provider.
- A read-merge-write sequence does not protect against an agent write at the same time.
- Transport-specific missing-file errors must map to one shared absent-target result.
