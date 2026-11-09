---
id: "02-deliver-announcements"
title: "Integrate Discord delivery and recovery"
status: done
wave: 2
depends_on:
  - "01-format-changelog"
plan: "plan.md"
requirements:
  - REQ-RELEASE-DISCORD-001
  - REQ-RELEASE-DISCORD-002
  - REQ-RELEASE-DISCORD-003
acceptance_criteria:
  - AC-RELEASE-DISCORD-001.1
  - AC-RELEASE-DISCORD-001.2
  - AC-RELEASE-DISCORD-001.3
  - AC-RELEASE-DISCORD-001.4
  - AC-RELEASE-DISCORD-002.2
  - AC-RELEASE-DISCORD-002.6
  - AC-RELEASE-DISCORD-003.1
  - AC-RELEASE-DISCORD-003.2
  - AC-RELEASE-DISCORD-003.3
  - AC-RELEASE-DISCORD-003.4
system_design:
  - ../../specs/release/system-design/discord-announcements.md
---

# Task 02: Integrate Discord delivery and recovery

## Summary

Connect the formatter to GitHub metadata and Discord through a reusable workflow.
Add automatic normal-release delivery to general, explicit recovery, and operator documentation.

## In scope

Exact-tag reads, PR author validation, secret handling, preview/create/update, confirmed and uncertain outcomes, and release job conditions.
Use only the general webhook secret. Send one compact message and retain its confirmed message ID for recovery.
Register the new test commands in existing CI/local script-test entry points where contributor-notification tests already run.
Document setup, missing secrets, preview, separate delivery failures, reruns, and backfill recovery.

## Out of scope

Releasing Kandev, provisioning a webhook, changing contributor notices, and posting to any Discord channel without explicit authorization.

## Acceptance

1. Eligible normal releases call Discord automatically after all required channels succeed, while every excluded mode sends nothing.
2. End-to-end tests prove whole-release counts, attribution, preview, exactly one POST per create invocation, safe updates, bounded 429 handling, and uncertain-delivery recovery.
3. Operator docs explain setup and recovery without claiming exactly-once delivery or concealing successful package publication behind a notification error.

## Verification

Run from the repository root after creating the referenced test files:

```bash
python3 .github/scripts/discord-release-content_test.py
python3 .github/scripts/announce-discord-release_test.py
python3 .github/scripts/announce-discord-release-workflow-contract_test.py
python3 .github/scripts/notify-release-contributors_test.py
python3 .github/scripts/notify-release-contributors-workflow-contract_test.py
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/announce-discord-release.py` and its `_test.py` (new)
- `.github/scripts/announce-discord-release-workflow-contract_test.py` (new)
- `.github/workflows/announce-discord-release.yml` (new), `.github/workflows/release.yml`, `.github/workflows/lint-action-pinning.yml`, and `Makefile`
- `docs/public/release-process.md` and `.agents/skills/release/SKILL.md`
- This plan, both work orders, and the paired draft specifications for final status updates

## Dependencies

[Task 01](task-01-format-changelog.md).

## Risks

Secret-bearing operations must use trusted helper code. Preview must work without the webhook secret.
Actions summaries can expire, so recovery can require reading the Discord channel directly.
An interrupted send cannot safely retry automatically. Test that failure explicitly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/discord-announcements.md).
- [Design](../../specs/release/system-design/discord-announcements.md), Entry selection through Documentation and alternatives.
- `.github/AGENTS.md`, `.github/scripts/notify-release-contributors.py`, and its workflow contract tests.
- `/release`, `/tdd`, and `/docs-maintainer` skills.

## Results

Implemented on 2026-10-25. Automatic delivery uses one compact message after all required publication jobs succeed. Standalone preview, acknowledged create, and verified update use the same helper. CI and `make test-scripts` run all three new suites.

Validation:

- Formatter: 8 tests passed. Delivery and recovery: 16 tests passed. Workflow contract: 4 tests passed, including the excluded-mode and dependency-result matrix.
- Contributor helper: 23 tests passed. Contributor workflow: 4 tests passed. Existing release workflow: 65 tests passed. Action-pinning tests: 9 passed.
- Public-doc tests: 63 passed; all 48 published pages validated. Catalog validation and specification lint passed. `git diff --check` passed.
- `zizmor .github/workflows/announce-discord-release.yml`: no findings. The required full workflow scan completed with 38 unsuppressed findings in existing code; none point to the new workflow or added release jobs. These existing findings keep the full scan nonzero.
- The existing release tests initially lacked `gpg`; running them through `nix-shell -p gnupg` passed. Zizmor ran through `nix-shell -p zizmor` because it was not on the default PATH.
- The production helper previewed published `v0.98.0` through real GitHub reads: 385 total changes, six highlights, correct category counts, and no unresolved credits. No Discord message was sent. An isolated temporary bare repository supplied release history because this shared checkout has an unrelated broken branch reference that prevents tag fetches.

Public setup and recovery docs and the release skill are updated. Prepared for PR review; no workflow was dispatched or release published. Existing workflow-security findings are tracked in Kandev subtask `4bb81949-c018-421a-9f85-8a10674a7695`, created after checking open PRs for overlap.


SDK integration adds `publish-plugin-sdk` and `stable-complete` to automatic
delivery and recovery-notice prerequisites. This preserves the requirement that
announcements describe complete Stable publication after the SDK work lands.
The workflow contract tests cover failed, skipped, and cancelled prerequisites.
