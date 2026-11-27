---
created: 2026-11-27
status: implemented
requirements:
  - REQ-EXECUTORS-RESUME-REMOTE-AUTH-001
system_design:
  - ../../specs/executors/system-design/resume-remote-auth.md
legacy_specs: []
---

# Restore remote credentials on resume

## Approach

Reuse the fresh-launch resolver after resume restores runtime metadata, before GitHub routing and environment construction. Preserve the current missing-secret policy and recorded workload identity. No new credential mechanism is needed.

## Work order

- [Restore and verify selected credentials](task-01-restore-selected-credentials.md)

## Verification and limits

Use dummy credentials through the real resume builder and scoped secret store. Check the resulting lifecycle environment definitions, executor eligibility, and managed-broker behavior. Run affected race tests, changed-code lint, documentation validation, and repository checks.

Authentication against a real provider, idle suspension, retained-PVC replacement, and shared deployment remain unverified. The runtime check uses an isolated local cluster and dummy credentials.

## Results

The focused race regressions, formatting, typecheck, full lint, and documentation checks passed. Manual local Kind verification confirmed selected credentials in a fresh process, a resumed process, the session auth file, and a real login shell after a controller restart. Pod, workspace, environment, executor, and profile identity were preserved. The disposable backend and cluster were removed.

The full executor race run failed three checkout-recovery tests that also failed with the original upstream resume source. The repository test run had other failures and its remaining long-running cases were stopped. See the [work order](task-01-restore-selected-credentials.md#results) for the evidence and limits.
