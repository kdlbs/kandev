---
created: 2026-10-25
status: in-progress
requirements:
  - REQ-AGENTS-PI-REMOTE-AUTH-001
  - REQ-AGENTS-PI-REMOTE-AUTH-002
system_design:
  - ../../specs/agents/system-design/pi-remote-auth.md
---

# Implementation Plan: Pi Remote Credentials

## Overview

Declare Pi's credential file, API-key variables and configuration bundle so
the Remote Credentials card offers them on every remote executor, and forward
the one missing variable over SSH. Preserve remote settings and models during
Kubernetes reconnects, and merge credentials with existing Kubernetes auth
files.

## Work orders

- [Task 01: Declare Pi remote credentials and configuration](task-01-declare-pi-remote-auth.md)

## Verification

- Unit: `go test ./internal/agent/agents -run PiACP` and
  `go test ./internal/agent/runtime/lifecycle -run SSHRemoteAgentEnv`.
- Kubernetes: `go test ./internal/agent/runtime/lifecycle -run
  'TestKubernetesCredentialUploader|TestKubernetesPodFileUploaderReadFile|TestKubernetesReadFileCommand|TestKubernetesFreshProvision' -count=1`.
- Web: `pnpm run i18n:check`, typecheck and the profile-edit component tests.

## Results

- `go test ./internal/agent/runtime/lifecycle -count=1` passed (216.990s).
- The focused Kubernetes uploader, read-command, and reconnect tests passed.
- `go test ./internal/agent/agents -run TestPiACP -count=1` passed.
- Scoped Go lint passed with 0 issues.
- The document catalog and full specification lint passed.
