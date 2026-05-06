---
status: draft
created: 2026-04-28
owner: cfl
---

# Rate-Limit Retry with Parsed Reset Time

## Why

When a subscription agent hits a provider rate limit, the current retry logic applies exponential backoff: 2 min → 10 min → 30 min → 2 hours. Rate-limit errors almost always include a precise reset timestamp in the error message (e.g. "resets at 4:00 AM", `Retry-After: 3600`, "try again in X minutes"). Ignoring this information means the agent may wait hours when it could have retried in seconds, or retries too early and burns another slot.

Parsing the reset time and scheduling the retry for `reset_time + 30s` makes wakeup recovery predictable and fast, without changing the safety net for transient errors that carry no timing information.

## What

### Parsing reset time from rate-limit errors

When `HandleWakeupFailure` is called with an error, the service checks whether the error is a rate-limit error and, if so, attempts to extract a reset timestamp from the message text:

- **"resets at HH:MM AM/PM"** — absolute wall-clock time on the current day (next occurrence if already past).
- **"Retry-After: N"** — N seconds from now (standard HTTP header value embedded in error text).
- **"try again in X minutes"** / **"try again in X seconds"** — relative duration from now.
- **"rate limit exceeded … reset_time: <unix timestamp>"** — Unix epoch seconds.

A buffer of **30 seconds** is added to any parsed reset time to avoid exact-second races with the provider's internal counter.

If the error message matches none of these patterns, or if the parsed time is in the past after applying the buffer, the existing exponential backoff is used unchanged.

### Retry scheduling

- `parseRateLimitResetTime(errorMsg string) (*time.Time, error)` returns the absolute UTC retry time (including buffer), or `nil` if not parseable.
- `HandleWakeupFailure` calls this parser before calling `scheduleRetry`. If a reset time is found, the wakeup is scheduled for that time regardless of `RetryCount` position in the backoff table.
- `RetryCount` is still incremented on every retry, including rate-limit retries. This ensures the escalation path (`MaxRetryCount`) still applies if rate-limit errors repeat indefinitely.
- The parsed reset time is logged alongside the existing retry log fields for observability.

### Rate-limit error detection

An error is treated as a rate-limit error when its message contains at least one of:

- `"rate limit"` (case-insensitive)
- `"rate_limit"`
- `"429"`
- `"too many requests"` (case-insensitive)
- `"quota exceeded"` (case-insensitive)

Detection is separate from reset-time parsing: a rate-limit error with no parseable reset time still falls through to normal backoff.

### Observability

`scheduleRetry` gains an optional `source` field in its log line:

- `source: "rate_limit_parsed"` when the reset time came from parsing.
- `source: "backoff"` for the existing exponential path.

The parsed reset time is logged as `parsed_reset_at` (UTC).

## Scenarios

- **GIVEN** a wakeup fails with `"rate_limit_error: resets at 4:00 AM UTC"`, **WHEN** `HandleWakeupFailure` is called at 3:45 AM UTC, **THEN** the wakeup is scheduled for `04:00:30 AM UTC` (parsed time + 30s buffer). The exponential backoff table is not consulted.

- **GIVEN** a wakeup fails with `"429 Too Many Requests, Retry-After: 3600"`, **WHEN** `HandleWakeupFailure` is called, **THEN** the wakeup is scheduled for `now + 3600s + 30s`.

- **GIVEN** a wakeup fails with `"try again in 5 minutes"`, **WHEN** `HandleWakeupFailure` is called, **THEN** the wakeup is scheduled for `now + 5m30s`.

- **GIVEN** a wakeup fails with a generic network timeout (no rate-limit keywords), **WHEN** `HandleWakeupFailure` is called, **THEN** the existing exponential backoff applies unchanged.

- **GIVEN** a wakeup fails with `"rate limit exceeded"` but no parseable reset time in the message, **WHEN** `HandleWakeupFailure` is called, **THEN** the existing exponential backoff applies (fallback path).

- **GIVEN** a rate-limit error causes repeated retries up to `MaxRetryCount`, **WHEN** the final retry also fails, **THEN** `escalateFailure` is called as normal — the rate-limit path does not suppress escalation.

## Out of scope

- Modifying `MaxRetryCount` for rate-limit errors (it stays at 4).
- Surfacing the parsed reset time in the UI or inbox.
- Per-provider configuration of reset-time parsing patterns.
- Handling `Retry-After` as an HTTP response header (this spec covers only error message text).
