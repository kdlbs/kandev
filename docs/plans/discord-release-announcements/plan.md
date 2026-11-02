---
created: 2026-10-23
status: implemented
requirements:
  - REQ-RELEASE-DISCORD-001
  - REQ-RELEASE-DISCORD-002
  - REQ-RELEASE-DISCORD-003
system_design:
  - ../../specs/release/system-design/discord-announcements.md
legacy_specs: []
---

# Discord release announcements implementation plan

## Overview

Build the grouped content formatter first, then integrate delivery and recovery with the Stable workflow.
The [requirements](../../specs/release/requirements/discord-announcements.md) define one compact announcement with whole-release counts and the exact GitHub release link.
The [design](../../specs/release/system-design/discord-announcements.md) defines counting, formatting, one webhook, and recovery.
Both work orders are done. The workflow and helpers are implemented locally and await normal repository delivery.

## Scope

Include Stable completion gates, whole-release category counts, attributed highlights, emojis, one GitHub release link, preview, and explicit recovery.
Exclude Nightlies, AI summaries, Kandev UI, automatic crossposting, and an independent delivery database.
Exclude full Discord changelogs, attachments, the website changelog link, and unsigned-installer copy.

## Technical approach

The new Python formatter consumes published note sections and resolved GitHub credits.
The formatter counts the complete release before selecting highlights. The announcement helper enriches displayed credits and sends one message through the general webhook.
The reusable workflow owns preview/create/update modes, the secret boundary, and serial delivery.
The existing release workflow calls it after all required publication jobs succeed.
Automatic reruns and backfills defer to explicit recovery. This avoids an unsupported exactly-once claim.

## Message contract

Illustrative content, with placeholder entries and handles:

> **🎉 Kandev vX.Y.Z is available!**
>
> **✨ Features (1)**
> - Add a new feature ([#123](https://github.com/kdlbs/kandev/pull/123)) by @contributor
>
> **🐛 Fixes (1)**
> - Correct a reported issue ([#124](https://github.com/kdlbs/kandev/pull/124)) by @maintainer
>
> View the full release on GitHub
>
> 2 highlights shown · 2 changes in the full release

Actual credits link to GitHub profiles. The actual GitHub release link names the selected tag.
Each category heading shows its total entry count without a PR-type prefix.
The count includes entries omitted from the excerpt, not just the visible entries.
The compact format shows up to three Features entries and three Fixes entries, plus counts for other populated categories.
The release link opens GitHub. Do not include a website changelog link, full Discord series, attachment, attachment explanation, or unsigned-installer notice.
Show the total release count and distinguish it from the displayed highlight count.
The category emojis are ✨ Features, 🐛 Fixes, ⚡ Performance, ♻️ Refactoring, and 📚 Documentation.
Discord owns desktop and mobile rendering. No Kandev UI components or CSS change.

## Tests

New `.github/scripts/discord-release-content_test.py`:

- `test_grouped_entries_and_links`: AC-002.1 and AC-002.3, including category names, total entry counts, and no PR-type prefixes in headings.
- `test_credits_include_maintainers_and_fallback`: AC-002.2.
- `test_counts_cover_complete_release_before_selection`: AC-002.1 and AC-002.4, including unchanged totals after trimming and a fixture larger than one page.
- `test_unicode_limits_and_disabled_mentions`: AC-002.4 and AC-002.5.
- `test_compact_format_and_emojis`: AC-002.6 and AC-002.7, including bounded selection, full counts, the GitHub-only release destination, and excluded copy.

New `.github/scripts/announce-discord-release_test.py`:

- `test_release_selection_and_attribution`: AC-001.2 and AC-002.2, including invalid PR links and GitHub failures.
- `test_preview_has_no_discord_writes`: AC-003.1.
- `test_confirmed_and_uncertain_sends`: AC-001.3 and AC-003.2.
- `test_update_requires_matching_message`: AC-003.3.
- `test_create_requires_explicit_recovery_acknowledgement`: AC-003.4.
- `test_single_webhook_single_message`: AC-002.6, proving that no full-release webhook, attachment upload, or extra POST occurs.

New `.github/scripts/announce-discord-release-workflow-contract_test.py` covers AC-001.1 through AC-001.4 and the standalone input/secret contract.
Use the complete `AC-RELEASE-DISCORD-*` IDs in implementation tests. Short IDs above refer to this document's three requirements.

## End-to-end evidence

Exercise the helper command with fake GitHub and Discord transports, covering preview, create, update, rate limits, and a lost POST response.
This is an external-message flow, so a Playwright test would not exercise the relevant boundary.
The user authorized live previews and later selected compact-only delivery.
On 2026-10-25, fresh v0.98.0 release notes confirmed 57 Features, 308 Fixes, 7 Performance, 6 Refactoring, and 7 Documentation entries, totaling 385.
The existing Features/Fixes counts already matched. The revised post explicitly separates the 385-entry total from its six displayed highlights.
The post retains only the GitHub release destination. All 11 full-changelog test messages were deleted and verified absent at the user's request.
Read-back checks verified message content, no attachments, and no mentions. Credentials are excluded from this package.
These previews used temporary scripts, not the planned production helper or workflow. Further live sends require explicit authorization.

## Work orders

1. [Task 01: Format grouped attributed changelogs](task-01-format-changelog.md). Done.
2. [Task 02: Integrate delivery and recovery](task-02-deliver-announcements.md). Done. Depends on Task 01.

Execute sequentially using TDD. Task files contain exact checks and owned paths.

## Verification results

Implemented on 2026-10-25. Automatic delivery uses one compact message after all required publication jobs succeed. Standalone preview, acknowledged create, and verified update use the same helper. CI and `make test-scripts` run all three new suites.

Validation:

- Formatter: 8 tests passed. Delivery and recovery: 16 tests passed. Workflow contract: 4 tests passed, including the excluded-mode and dependency-result matrix.
- Contributor helper: 23 tests passed. Contributor workflow: 4 tests passed. Existing release workflow: 65 tests passed. Action-pinning tests: 9 passed.
- Public-doc tests: 63 passed; all 48 published pages validated. Catalog validation and specification lint passed. `git diff --check` passed.
- `zizmor .github/workflows/announce-discord-release.yml`: no findings. The required full workflow scan completed with 38 unsuppressed findings in existing code; none point to the new workflow or added release jobs. These existing findings keep the full scan nonzero.
- The existing release tests initially lacked `gpg`; running them through `nix-shell -p gnupg` passed. Zizmor ran through `nix-shell -p zizmor` because it was not on the default PATH.
- The production helper previewed published `v0.98.0` through real GitHub reads: 385 total changes, six highlights, correct category counts, and no unresolved credits. No Discord message was sent. An isolated temporary bare repository supplied release history because this shared checkout has an unrelated broken branch reference that prevents tag fetches.

Public setup and recovery docs and the release skill are updated. Prepared for PR review; no workflow was dispatched or release published. Existing workflow-security findings are tracked in Kandev subtask `4bb81949-c018-421a-9f85-8a10674a7695`, created after checking open PRs for overlap.

After the compact-only revision on 2026-10-25, catalog validation, specification lint, full work-order reference checks, document links, and whitespace checks passed again.
The earlier live preview read-back verified the sample messages. The implementation checks above now cover the production helper and workflow contract.

Design checks passed on 2026-10-23:

- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- The repository `validateCoverage` API accepted both work orders and their references. A synthetic non-exempt path enabled full checks for this documentation-only package.
- Local document links and untracked-file whitespace checks passed. The release catalog discovers both new specification files.
- `git diff --check -- docs/specs docs/plans/discord-release-announcements` passed. Git status confirms the complete uncommitted package.

## Risks and rollout

- Some release entries lack resolvable PR authors. Display the explicit fallback and surface the warning in preview.
- Webhooks cannot prove duplicate-free delivery after a lost response. Reruns and backfills require channel inspection and manual recovery.
- Counts must use the complete release before selection, independent of API pagination or the display budget.
- The operator must configure the general webhook secret. Keep credentials outside the repository and conversation.
