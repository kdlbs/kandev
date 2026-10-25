---
id: "01-refresh-stable-pins"
title: "Refresh stable managed runtime pins"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.9
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
---

# Task 01: Refresh stable managed runtime pins

## Summary

Refresh the trusted managed npm runtime catalogue from each package's stable
`latest` metadata through the existing updater. Keep the change reviewable and
do not install or activate runtimes.

## Acceptance

- Every catalogue entry uses the stable `latest` version reported by npm.
- The updater validates the catalogue before writing it and does not partially
  update entries on lookup or validation failure.
- The change does not install, activate, or merge a runtime update.

## Verification

```bash
node scripts/update-agent-runtime-pins.mjs
```

## Results

The updater refreshed all seven catalogue entries. A fresh follow-up run
confirmed that the catalogue still matches npm's stable `latest` metadata:

- Claude ACP: `0.81.2`
- Codex ACP: `1.13.1`
- Muse ACP: `0.7.0`
- GitHub Copilot: `1.0.88`
- Gemini CLI: `0.61.0`
- OpenCode: `1.18.32`
- Pi ACP: `0.0.34`

No runtime was installed or activated. The public agent guide documents the
reviewed-default and update behavior without hard-coding current version values,
so it does not need a change for this refresh.
