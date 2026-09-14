---
id: "04-public-docs"
title: "Document variation fallback"
status: done
wave: 3
depends_on: ["01-runtime-resolution", "02-frontend-advisory"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.6
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
---

# Task 04: Document Variation Fallback

## Summary

Update the public profile and executor guides with the unique-variation rule.
Give users enough detail to predict unique and ambiguous results.

## In scope

- Document the four-step launch order.
- Add one unique and one ambiguous example.
- State that labels remain opaque and saved profiles do not change.
- Run the public-doc validation commands.

## Out of scope

- Release notes and changelog generation.
- Internal implementation detail beyond user-visible behavior.
- New screenshots or media.

## Acceptance

- Both guides describe the same precedence and ambiguity rule.
- The docs state that the executor is authoritative and the profile stays
  unchanged.
- Public-doc validators pass.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `docs/public/executors.md`

## Dependencies

Tasks 01 and 02.

## Risks

- Examples can imply that Kandev understands `1m` or `fast`. The docs must state
  that it compares only the ID shape and candidate count.

## Parallelism

`parallel-safe` with Task 03 after Tasks 01 and 02.

## Inputs

- Final runtime precedence and warning behavior.
- Existing host-probe and remote-executor guide sections.

## Results

Updated the public agent-profile and executor guides with the four-step
resolution order, unique and ambiguous examples, opaque variation matching,
and the unchanged saved-profile rule. Public-document tests passed (61) and
the validator checked 46 published pages.
