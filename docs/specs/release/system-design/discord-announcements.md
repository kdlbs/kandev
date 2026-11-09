---
status: current
system: release
requirements:
  - REQ-RELEASE-DISCORD-001
  - REQ-RELEASE-DISCORD-002
  - REQ-RELEASE-DISCORD-003
---

# Discord release announcements

## Boundary

A GitHub Actions helper sends one compact announcement through the general-channel Discord webhook.
The release system owns selection, whole-release counting, formatting, and recovery. No Kandev runtime service is involved.
The user selected one compact message linked to the exact GitHub release. No full Discord series, website changelog link, or attachment is included.
Operator-assisted recovery handles reruns, backfills, and uncertain delivery.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-RELEASE-DISCORD-001 | Workflow integration, failure and recovery |
| REQ-RELEASE-DISCORD-002 | Entry selection, presentation, security |
| REQ-RELEASE-DISCORD-003 | Preview, failure and recovery |

## Components

- Existing `.github/workflows/release.yml` calls a new `.github/workflows/announce-discord-release.yml`.
- New `.github/scripts/announce-discord-release.py` owns the Python standard-library entry point, GitHub reads, Discord requests, and summaries.
- New `.github/scripts/discord-release-content.py` owns parsing, attribution assembly, and payload formatting without network access.
- Existing `cliff.toml` defines the published changelog categories and filtering. Discord does not regenerate a second release history.

Follow the existing contributor notification helper's release validation and GitHub argument-array conventions.
Do not apply its maintainer or bot exclusion rules to changelog attribution.
Use focused shared code only where it reduces duplication without changing contributor-notification behavior.

## Entry selection

Resolve an exact `vX.Y.Z` tag through the GitHub release API. Reject unpublished, draft, or prerelease results.
Bind release ID, tag, publication date, body, and URL once per run. Never fall back to latest.
Read all bullet entries within the selected release's complete published changelog sections before selecting highlights.
Use the complete release body, not a previous Discord message, an excerpt, or the last page of an API response.
If a metadata API paginates, consume its complete result before using it. Count changelog entries independently of PR metadata availability.
Preserve entry wording and section order. Treat HTML and arbitrary Markdown as data.

Map Features to `feat`, Bug Fixes to `fix`, Performance to `perf`, Refactoring to `refactor`, and Documentation to `docs`.
Preserve other populated sections as Other changes. Continue the existing omission of release, chore, ci, and test entries from published notes.
Do not copy the unsigned macOS/Windows installer notice or its bypass instructions into Discord. The linked release notes retain those details.

For each displayed entry with a same-repository PR link, fetch the PR author and validate that the PR is merged into this repository.
Prove the merge commit is an ancestor of the selected release tag, following the existing contributor helper.
Cache repeated PR lookups. Credit all distinct validated PR authors when an entry references several PRs.
Use GitHub profile links for credits, including maintainers and bot accounts.
For an entry without valid PR metadata, preserve an existing syntactically valid GitHub attribution from the published notes.
For an unlinked entry, a unique exact-subject commit match can supply GitHub's associated author after proving membership in the selected tag.
Otherwise use the explicit unavailable-attribution label. Never infer a handle from a Git author name or email.
Unexpected GitHub or Git failures stop before sending. Known deleted or unavailable authors generate warnings instead of invented credits.

## Presentation

Include the version, publication date, total release count, counts for every populated category, and the exact GitHub release link.
Use category emojis: Features ✨, Fixes 🐛, Performance ⚡, Refactoring ♻️, Documentation 📚, and Other changes 🛠️.
Format entry-group headings as `<emoji> <category name> (<total entry count>)`, without PR-type identifiers.
Count each entry once regardless of its PR links or contributors. Compute category counts and their sum before selecting or trimming highlights.
Counts do not change when fewer entries fit. Show the displayed highlight count separately from the whole-release total.

Use one compact message with a celebratory heading, up to three Features entries, and up to three Fixes entries.
If both categories are absent, show up to three entries from the first populated category.
Retain source order within each category. Other categories remain visible through their counts.
Keep embeds single-column. Every displayed entry retains its PR links and linked contributor credits.

Keep a 2,500-character total text budget, including content, embeds, credits, counts, and links.
Drop whole highlight entries as needed while preserving counts and the exact GitHub release link.
An individually oversized entry is skipped in the preview selection. It remains included in its category count.
Omitted entries remain accessible in the GitHub release notes. Never send additional messages or attachments for overflow.
Do not include the website changelog link, a full-Discord-message link, attachment explanation, or unsigned-installer notice.

Validate Discord's 2,000-character content limit, 256-character embed titles, 4,096-character descriptions, ten-embed limit, and 6,000-character aggregate embed limit.
Measure final escaped payloads conservatively in UTF-16 units. Validate the message before sending.
Use a footer such as “6 highlights shown · 385 changes in the full release”. Do not display internal release IDs.

## Workflow integration

Add `announce-discord` beside `notify-contributors`, with the same publication dependencies.
Require `!cancelled()`, manual Stable mode, and explicit success for prepare, all publication jobs including `publish-plugin-sdk`, and `stable-complete`.
Exclude dry runs, desktop validation, nonempty `backfill_tag`, and `github.run_attempt != 1`.
There is no per-release opt-in checkbox. Normal eligible releases announce automatically after setup.
Reruns and backfills report that announcement recovery requires the standalone workflow.
This conservative rule avoids automatic reposting without introducing durable delivery storage.

The standalone workflow accepts required `release_tag`, `mode` (`preview`, `create`, or `update`), and optional `message_id`.
Default to preview. Require `message_id` for update and reject it for create.
Manual create includes explicit acknowledgement that the operator checked publication channels and searched for an existing announcement.
Preview needs no Discord secret. Create and update use `DISCORD_GENERAL_WEBHOOK_URL` in the existing `release` environment.
Do not define or call a full-release webhook. There is no destination selection or multipart receipt contract.
Use a separate delivery job for secret-bearing operations. Apply only `contents: read` permissions.
Use one repository-wide Discord notification concurrency group with non-cancelling queued runs, separate from the caller's release lock.
Read helpers from the trusted workflow revision, not from the selected historical tag. Record workflow and checkout identities separately.

## Preview

Run GitHub selection and the same formatter used for sending.
Write the exact compact payload, whole-release counts, displayed count, and attribution warnings into a preview artifact and job summary.
Preview artifacts contain public release data only. Never include credentials or webhook URLs.
The implementation tests exercise the helper entry point and fake GitHub and Discord transports.
An optional private-channel smoke test occurs only after an operator explicitly authorizes that message.

## Failure and recovery

Create with `wait=true`. Validate the returned message ID and channel ID before reporting confirmed delivery.
Record the confirmed message ID and link in the Actions summary.
For update, fetch the supplied message through the general webhook and verify its version and exact GitHub release link before PATCH.
Send `attachments: []` on updates so a revised test message does not retain an earlier attachment.
Missing, foreign, or mismatched messages stop without a new POST.

Retry an explicit Discord 429 according to `retry_after`, for at most three total attempts within a two-minute deadline.
Do not automatically retry a POST after a timeout, connection loss, or 5xx response. Report delivery as uncertain.
Authentication and payload errors fail immediately. Missing credentials produce a clear configuration error, not a silent skip.
The operator checks the channel, supplies the existing message ID for update, or explicitly requests create when no message exists.
Do not promise exactly-once delivery: a webhook cannot search channel history, and a successful send can lose its response.
Actions summaries support recovery but are not a durable deduplication ledger. New manual creates can duplicate a message if the operator skips inspection.
Delivery failure can make the notification job red while all publication jobs remain successful.

## Security and setup

Create one webhook for general and store its URL as the environment secret `DISCORD_GENERAL_WEBHOOK_URL`.
An administrator performs this setup during implementation rollout. Do not request the secret in chat or commit it.
Validate HTTPS and the expected Discord API webhook path. Reject redirects and custom hosts for live delivery.
Inject fake transports in tests instead of weakening production URL validation.
Pass release data through JSON and argument arrays, never through shell interpolation.
Set `allowed_mentions` to disable all parsing on both POST and PATCH. Escape Markdown in descriptions and contributor labels.
Redact the webhook token and URL from errors. Never log raw HTTP exceptions that contain the request URL.

## Documentation and alternatives

Implementation updates `docs/public/release-process.md` with setup, preview, recovery, and the separate notification outcome.
The `/release` skill contains matching operator guidance.
No ADR is needed for this local release helper. The design preserves the rationale and limits.
Alternatives for later consideration are a dedicated discussion thread or a bot with history access.
They add permissions or lifecycle behavior and are outside this first package.

## References

Checked on 2026-10-23:

- [Discord webhook API](https://docs.discord.com/developers/resources/webhook)
- [Discord embed limits](https://docs.discord.com/developers/resources/message#embed-limits)
- [Discord Markdown](https://support.discord.com/hc/en-us/articles/210298617-Markdown-Text-101-Chat-Formatting-Bold-Italic-Underline)
- [Discord components](https://docs.discord.com/developers/components/reference)
- [GitHub pull request API permissions](https://docs.github.com/en/rest/pulls/pulls#get-a-pull-request), checked on 2026-10-25: the selected single-PR read supports `contents: read`.
- [Contributor notification design](contributor-notifications.md)
