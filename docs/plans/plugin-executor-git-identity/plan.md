---
created: 2026-10-25
status: implemented
requirements:
  - REQ-EXECUTORS-PLUGIN-002
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
legacy_specs: []
---

# Implementation Plan: Plugin executor Git identity

## Overview

Apply the Git name and email set in a plugin executor profile's Remote
Credentials card, which are saved and passed into launch metadata but never
applied, against the executor-owned
[requirements](../../specs/executors/requirements/remote-executor-plugins.md)
and [design](../../specs/executors/system-design/remote-executor-plugins.md).

## Evidence and root cause

`git_user_name` and `git_user_email` are Kandev-owned plugin profile keys, and
`applyProfileConfigToMetadata` copies them into launch metadata. SSH, Docker,
Kubernetes and Sprites apply them through `{{git.identity_setup}}` in their
prepare scripts (`scriptengine.GitIdentityProvider`). Plugin executors run no
prepare script, and `PluginRemoteExecutor` reads neither key, so the
environment's global Git configuration stays unset.

## Scope

- `applyPluginExecutorGitIdentity` in `executor_plugin_credentials.go`, called
  after the credential upload in `CreateInstance` and in retained attach.
- Requirement AC-EXECUTORS-PLUGIN-002.8, the system design paragraph, and
  `docs/public/executors.md`.

Out of scope: a prepare-script mechanism for plugin executors.

## Work orders

- [Task 01: Apply the profile Git identity on plugin executors](task-01-apply-git-identity.md)

## Verification

- `go test -tags fts5 ./internal/agent/runtime/lifecycle` passes as a whole.
- `executor_plugin_git_identity_test.go` runs the command through a real shell
  and `git`, reads the identity back from the remote home's `.gitconfig`,
  checks that neither value is in a command line, and checks that nothing runs
  when either value is missing. Dropping the email from the command makes it fail.
