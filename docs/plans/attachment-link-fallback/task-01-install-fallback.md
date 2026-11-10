---
id: "01-install-fallback"
title: "Restore portable attachment installation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.1
  - AC-TASKS-PROMPT-ATTACHMENTS-001.5
  - AC-TASKS-PROMPT-ATTACHMENTS-001.7
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Restore portable attachment installation

## Summary

Issue #4540 fails because non-collision `os.Link` errors abort installation.
The typed HTTP error then bypasses the handler's filesystem-error logger.
Keep hard-link installation and add an exclusive-copy fallback with regression
coverage for successful delivery and incomplete-copy rejection.

## In scope

Agentctl attachment installation, focused tests, and materialization design.

## Out of scope

Other `os.Link` callers, UI changes, and new security boundaries.

## Acceptance

- Unsupported hard links still permit a complete, private attachment installation.
- Existing files and destination symlinks survive naming collisions unchanged.
- Failed copies never report success; partial files are removed and causes are logged safely.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api -run 'Test(MaterializeAttachment|InstallMaterializedAttachment|CopyMaterializedAttachment|WriteMaterializedAttachment)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/api/attachments.go apps/backend/internal/agentctl/server/api/attachments_test.go apps/backend/internal/agentctl/server/api/attachments_install_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/attachments.go`
- `apps/backend/internal/agentctl/server/api/attachments_test.go`
- `apps/backend/internal/agentctl/server/api/attachments_install_test.go`
- `docs/specs/tasks/system-design/prompt-attachments.md`

## Dependencies

None.

## Risks

No real EdenFS/FUSE smoke test; unsupported links use forced syscall errors.
Write and close errors are injected at the stream boundary. Cleanup is proved
with real filesystem read failures and the HTTP endpoint.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/prompt-attachments.md), materialization and failure modes.
- [System design](../../specs/tasks/system-design/prompt-attachments.md), materialization and delivery.
- Existing attachment handler and concurrent-installation tests.

## Results

All verification commands passed. Focused race tests passed locally and during
independent parent validation. Scoped lint reported zero issues; documentation
catalog and specification lint passed. The regression first failed for forced
EPERM, ENOTSUP, and EXDEV, then passed after the fallback. Normal commit hooks
passed without bypass. Public docs need no change because existing attachment
behavior is restored without new configuration or API fields.
