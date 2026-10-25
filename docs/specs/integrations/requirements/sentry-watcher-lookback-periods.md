---
status: draft
system: integrations
created: 2026-09-18
owners:
  - kandev
---

# Sentry Watcher Lookback Period Requirements

## Overview

A Sentry lookback period is the selected maximum age of an issue's first-seen time. A Sentry issue watch exposes that selection, and the manual issue browser exposes the same selection through the same backend search path, so this capability covers both surfaces and the integration system owns their provider-facing contract.

## Terminology

- **Lookback period:** The selected maximum age of an issue's first-seen time for a Sentry issue search.
- **Lookback token:** A lookback period in Sentry's relative-duration syntax, one of `1h`, `24h`, `7d`, `14d`, or `30d`.
- **Supported lookback syntax:** An integer from `1` to `3650`, written without a leading zero, followed by `h`, `d`, or `w`, such as `1h`, `7d`, or `2w`. A value in any other form or above that bound, such as `30m`, `0h`, `007h`, `1.5h`, or `9999w`, is outside the syntax.
- **Issue browser:** The manual Sentry issue search dialog.
- **`statsPeriod` request parameter:** An endpoint-specific Sentry window parameter. One issue endpoint accepts only `''`, `24h`, and `14d` there, while another accepts any relative duration and uses it as the request time range.

## Requirements

### REQ-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001: Sentry lookback fidelity

**Intent:** A selected lookback must limit matching issues on every Sentry search Kandev issues, and must never be forwarded through a provider parameter that rejects it.

**User story:** As a user configuring a Sentry issue watch, I want each offered lookback period to save and filter matching issues, so that the watch creates tasks from only the intended interval.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1:** When a user creates or updates a Sentry issue watch with any offered lookback token (`1h`, `24h`, `7d`, `14d`, or `30d`), the system shall retain the selected value and limit matching issues to that interval.
- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.2:** When Kandev issues a Sentry issue search with a selected lookback, the system shall express issue eligibility through Sentry's search query, and shall send that lookback as the `statsPeriod` request parameter only to an endpoint whose accepted values include it.
- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.3:** When an existing watch stores one of the offered lookback tokens, the system shall continue to load and poll it with that stored interval, without migration or alteration.
- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.4:** When a create or update request supplies a non-empty lookback value outside Kandev's supported lookback syntax, the system shall reject the request rather than store a search that has no age limit, while an empty or absent lookback remains accepted and applies no age constraint.
- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.5:** When the issue browser searches with a selected lookback, the system shall limit returned issues to that lookback interval.
- **AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.6:** When a stored lookback value is outside Kandev's supported lookback syntax, the system shall fail that watch's poll with a recorded error and create no tasks from it, rather than searching without an age constraint.

## Out of scope

- Adding lookback tokens beyond the offered set, or accepting lookback values that are not whole hours, days, or weeks, such as minutes or seconds.
- Unifying the two issue endpoints' accepted value sets or their per-issue event-count windows.
- Auditing or rewriting lookback values already stored on rows the API does not subsequently write.
- Changing issue-watch filters other than the lookback period.
