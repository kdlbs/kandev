---
id: "03-integration-and-verification"
title: "Verify durable cross-turn attribution"
status: done
wave: 3
depends_on: ["01-backend-turn-snapshot", "02-frontend-turn-attribution"]
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 03: Verify Durable Cross-turn Attribution

## Acceptance

- Focused backend and frontend suites pass together after formatting.
- Refresh/API hydration continues to carry turn snapshots through the existing turn metadata contract.
- Full repository format, typecheck, test, and lint checks pass or any unrelated blocker is documented precisely.

## Verification

```bash
rtk make fmt
rtk node apps/web/scripts/generate-release-notes.mjs
rtk node apps/web/scripts/generate-changelog.mjs
rtk make typecheck
rtk make test
rtk make lint
```

Run from the repository root.

## Files

- Backend and frontend files from tasks 01 and 02
- `docs/specs/ui/acp-model-configuration-summary.md`
- `docs/decisions/2026-07-18-turn-configuration-snapshots.md`

## Inputs

- Completed task 01 and task 02 changes.
- Existing turn API/boot/WebSocket serialization tests.

## Output Contract

Report all commands and results, integration gaps, formatter changes, unresolved failures, and final task/plan status.
