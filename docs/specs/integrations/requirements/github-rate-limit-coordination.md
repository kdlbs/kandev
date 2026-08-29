---
status: active
system: integrations
created: 2026-08-29
owners:
  - kandev
---

# GitHub Rate-Limit Coordination Requirements

## Overview

Kandev coordinates GitHub provider traffic so background synchronization cannot
starve interactive agents and operators. The integration records the provider
signals it observes, distinguishes primary quota exhaustion from secondary
throttling, and exposes its locally enforced state without issuing another
GitHub request.

## Terminology

- **Primary limit:** A GitHub resource bucket whose response reports
  `X-RateLimit-Remaining: 0` and a reset time.
- **Observed secondary throttle:** A 403 or 429 rate/abuse response without a
  zero primary remainder. GitHub exposes no authoritative status endpoint for
  this condition.
- **Retry source:** Either GitHub's `Retry-After` signal or Kandev's
  conservative fallback when GitHub omits that signal.
- **Quota principal:** The upstream human login or GitHub App installation that
  shares one provider budget across Kandev workspaces.

## Requirements

### REQ-INTEGRATIONS-GITHUB-RATE-001: Distinct provider failure states

**Intent:** Agents and background services need remediation that matches the
provider failure instead of treating every 403 as primary quota exhaustion.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-RATE-001.1:** When a response reports zero primary
  remaining quota, the integration shall classify primary exhaustion and use
  the provider reset time.
- **AC-INTEGRATIONS-GITHUB-RATE-001.2:** When a 403 or 429 contains a rate or
  abuse signal while primary remaining is positive or unknown, the integration
  shall classify an observed secondary throttle and preserve the healthy
  primary snapshot.
- **AC-INTEGRATIONS-GITHUB-RATE-001.3:** When GitHub reports invalid
  credentials/access or a missing repository, branch, path, or ref, the
  integration shall classify those conditions separately from rate limiting.
- **AC-INTEGRATIONS-GITHUB-RATE-001.4:** When GitHub supplies `Retry-After`,
  `X-RateLimit-Remaining`, or `X-RateLimit-Reset`, the integration shall honor
  those signals without using a reset header alone as evidence of primary
  exhaustion.
- **AC-INTEGRATIONS-GITHUB-RATE-001.5:** When a successful provider response is
  accepted before a locally estimated secondary retry time, the integration
  shall clear the observed secondary throttle early.

### REQ-INTEGRATIONS-GITHUB-RATE-002: Principal-wide provider admission

**Intent:** Background GitHub work must leave provider capacity for interactive
agents and operators.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-RATE-002.1:** When multiple workspaces use the same
  GitHub human login, their Kandev-routed requests shall share one observed
  budget and throttle state.
- **AC-INTEGRATIONS-GITHUB-RATE-002.2:** When background work reaches the
  configured primary reserve or an active provider retry window, Kandev shall
  defer it before issuing a provider request.
- **AC-INTEGRATIONS-GITHUB-RATE-002.3:** When interactive and background work
  are both eligible, interactive work shall have admission priority.

### REQ-INTEGRATIONS-GITHUB-RATE-003: Safe workflow-sync retry

**Intent:** A failing synchronized workflow source must not create repeated
provider bursts.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-RATE-003.1:** When automatic workflow sync fails
  transiently, Kandev shall persist equal-jitter exponential backoff with a
  one-hour cap and shall make no request before the stored next attempt.
- **AC-INTEGRATIONS-GITHUB-RATE-003.2:** When GitHub provides a later retry or
  reset time, that time shall be a lower bound for the next automatic attempt.
- **AC-INTEGRATIONS-GITHUB-RATE-003.3:** When a workflow-sync target has an
  invalid credential/access state or a missing target, automatic polling shall
  suspend after one actionable stored error instead of repeating the request.
- **AC-INTEGRATIONS-GITHUB-RATE-003.4:** When a user saves the sync
  configuration or explicitly selects Sync now, Kandev shall allow a recovery
  attempt and clear retry state after success.

### REQ-INTEGRATIONS-GITHUB-RATE-004: Zero-call agent visibility

**Intent:** An agent must be able to decide whether Kandev will admit a GitHub
call without consuming provider capacity to ask.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-RATE-004.1:** When an agent reads GitHub rate state,
  Kandev shall return observed core and GraphQL quota, observed secondary
  state, retry time and source, snapshot freshness, and interactive/background
  admission decisions without issuing a GitHub request.
- **AC-INTEGRATIONS-GITHUB-RATE-004.2:** When primary quota is full while an
  observed secondary throttle is active, the response shall preserve both
  facts and deny admission until Kandev's enforced retry time or an earlier
  successful response clears it.
- **AC-INTEGRATIONS-GITHUB-RATE-004.3:** When no bucket has been observed, the
  response shall mark it unknown rather than fabricate quota or make a
  provider request.

## Out of scope

- Claiming provider-authoritative secondary status or a guaranteed clear time
  that GitHub does not expose.
- Coordinating processes that use the same credential outside Kandev's GitHub
  clients.
- Adding a new settings UI for rate state.
