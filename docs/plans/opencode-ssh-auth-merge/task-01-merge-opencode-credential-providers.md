---
id: "01-merge-opencode-credential-providers"
title: "Merge OpenCode credential provider maps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.11
  - AC-EXECUTORS-SSH-EXECUTOR-001.12
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
---

# Task 01: Merge OpenCode Credential Provider Maps

## Summary

Add an agent-owned merge policy to the remote credential pipeline. Use it to preserve existing OpenCode provider logins on remote executors.

## In scope

- Add the internal credential-file conflict policy to agent and catalog models.
- Declare the JSON-object merge policy for OpenCode.
- Add target reads to persistent credential file transports.
- Keep isolated credential targets on the validated write path.
- Merge provider maps in the shared credential uploader.
- Preserve an existing target when a read or parse fails.
- Add focused unit and SSH integration tests.
- Update the public SSH executor reference.

## Out of scope

- A settings control for conflict behavior.
- Credential backups.
- Provider-specific deep merges below the top-level provider key.
- Remote file locking.

## Acceptance

- A copy preserves target-only providers and adds source-only providers.
- A source provider replaces a target provider with the same key.
- An unreadable or malformed existing target remains byte-for-byte unchanged.
- An unreadable source file reports a credential-copy error and does not write a target.
- A malformed source file reports a credential-copy error and does not write a target.

## Verification

```bash
cd apps/backend && go test ./internal/agent/agents ./internal/agent/remoteauth ./internal/agent/runtime/lifecycle -run 'Test(OpenCodeACPRemoteAuth|BuildCatalog|UploadCredentialFiles|SeedAgentSessionDir_OpenCode|SSHExecutorUploadCredentials|SSHFileUploader)' -count=1
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/opencode_acp.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`
- `apps/backend/internal/agent/remoteauth/catalog.go`
- `apps/backend/internal/agent/remoteauth/catalog_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/credential_uploader.go`
- `apps/backend/internal/agent/runtime/lifecycle/credential_uploader_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/agent_session_seeder.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials_remote_test.go`
- `docs/public/executors.md`

## Dependencies

None.

## Risks

- Each transport reports a missing file differently. The shared reader contract must normalize this condition.
- The source-wins collision rule can replace one existing provider login by design.

## Parallelism

`sequential`

## Inputs

- `REQ-EXECUTORS-SSH-EXECUTOR-001`
- `docs/specs/executors/system-design/ssh-executor.md`
- `docs/decisions/2026-09-05-agent-owned-credential-file-conflicts.md`
- GitHub issue `kdlbs/kandev#3382`
- Existing `FileUploader` and passthrough JSON merge patterns.

## Results

Implemented the agent-owned JSON-object conflict policy for OpenCode credentials. SSH and Sprites read persistent targets before the merge.

Isolated targets validate and write the source object. Malformed inputs leave an existing target unchanged, and unreadable or malformed sources report a credential-copy error before any write.

Public docs updated: `docs/public/executors.md` (reference).

Verification results:

- Focused Go tests: 19 passed in three packages.
- Specification linter: passed.
- Public documentation tests: 61 passed.
- Published documentation validator: 45 pages passed.
- Git diff check: passed.
