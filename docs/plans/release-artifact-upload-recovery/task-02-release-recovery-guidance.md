---
id: "02-release-recovery-guidance"
title: "Update release recovery guidance"
status: done
wave: 2
depends_on:
  - "01-resilient-artifact-uploads"
plan: "plan.md"
requirements:
  - REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001
acceptance_criteria:
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.4
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.5
system_design:
  - ../../specs/release/system-design/artifact-upload-recovery.md
---

# Task 02: Update release recovery guidance

## Summary

Document the artifact upload retry budget, desktop matrix behavior, and the
existing backfill path so maintainers can distinguish an exhausted retry budget
from a successful release. Keep the instructions consistent across public
release documentation, repository guidance, and the release skill.

## In scope

- Update `docs/public/release-process.md` with the operational retry and
  recovery behavior.
- Update the Release and Versioning guidance in `AGENTS.md`.
- Update `.agents/skills/release/SKILL.md` with the same contract.

## Out of scope

- New release commands or workflow inputs.
- Changes to public installation or runtime behavior.
- Repeating the full release workflow architecture in the public guide.

## Acceptance

- The public release guide states the bounded retry behavior and preserves the
  `backfill_tag` guidance for an exhausted or partial release.
- Internal release guidance states that retries remain fail-closed and that all
  desktop targets continue independently.
- The documentation validators and harness lint pass.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
git diff --check -- docs/public/release-process.md AGENTS.md .agents/skills/release/SKILL.md
```

## Files likely touched

- `docs/public/release-process.md`
- `AGENTS.md`
- `.agents/skills/release/SKILL.md`

## Dependencies

Task 01, so the documentation describes the final workflow contract.

## Risks

None.

## Parallelism

`sequential`

## Inputs

- `docs/specs/release/requirements/artifact-upload-recovery.md`
- `docs/specs/release/system-design/artifact-upload-recovery.md`
- `docs/decisions/0029-release-backfill-and-desktop-diagnostics.md`

## Results

Updated the public release process, root engineering guidance, and release
skill with the retry budget, missing-file behavior, desktop matrix isolation,
publication gate, failed-job rerun, and `backfill_tag` recovery guidance.

Verification passed:

- `node --test scripts/validate-public-docs.test.mjs`: 62 tests.
- `node scripts/validate-public-docs.mjs`: 47 pages.
- `python3 scripts/lint-harness-files.test.py`: 19 tests.
- `python3 .github/scripts/lint-harness-files.py --all`: 199 files.
- `git diff --check -- docs/public/release-process.md AGENTS.md .agents/skills/release/SKILL.md`.
