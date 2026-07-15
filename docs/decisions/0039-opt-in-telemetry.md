# 0039: Strictly opt-in product telemetry via PostHog EU

**Status:** accepted
**Date:** 2026-07-15
**Area:** backend, frontend

## Context

Kandev had no product telemetry, so decisions about which features to invest
in were made blind. Kandev is a local-first, open-source dev tool: its users
are exactly the audience most hostile to silent data collection, and 2025–26
incidents (GitHub CLI's default-on rollout, opt-out flags that leaked in
Strapi and Continue.dev) show that getting this wrong costs more trust than
the data is worth. The Go toolchain and Jan establish the defensible
precedent: opt-in only, anonymous, fully documented.

## Decision

Telemetry is **strictly opt-in** and lives in `internal/telemetry`:

- **Sink:** PostHog Cloud EU (`eu.i.posthog.com`, write-only project key
  baked in, overridable via `KANDEV_TELEMETRY_ENDPOINT` / `_API_KEY`).
  Events are sent in anonymous mode (`$process_person_profiles: false`)
  keyed by a random install UUID minted only after consent and deleted on
  opt-out. Chosen over Aptabase (weaker analysis), self-hosting (ops
  burden), and a custom endpoint (build cost) — 1M free events/month and an
  established OSS track record (Jan, n8n, Continue).
- **Collection:** a bus subscriber maps an allowlist of domain events
  (task/agent/turn/workspace/automation lifecycle) to name-only telemetry
  events — payloads are never forwarded, so titles/repos/paths cannot leak.
  Frontend UI events go through `POST /api/v1/telemetry/events`, validated
  server-side against a per-event property allowlist with enum-shaped
  values; the backend is the single enforcement point for every client and
  deploy mode.
- **Consent:** tri-state (`unasked/granted/denied`) on the install-wide
  `settings` table, exposed via `GET/PUT /api/v1/telemetry/consent`, a
  one-time onboarding step, and `Settings → System → Telemetry`.
- **Kill switches:** `DO_NOT_TRACK=1` and `KANDEV_TELEMETRY=off` win over
  everything; with either set the service starts no goroutines and
  subscribes to nothing. `profiles.yaml` forces `off` in dev and e2e.
  `KANDEV_TELEMETRY_DEBUG=1` logs every outgoing payload.
- **Delivery:** in-memory queue, batched flush, fail-silent, no disk
  persistence, drops on overflow — telemetry can never block product flows.

The full public contract (every event and property) is
`docs/public/telemetry.md`; it must be updated in the same PR as any
allowlist change.

## Consequences

The team gets adoption and feature-usage signal from consenting installs
with a privacy story that survives scrutiny: nothing identifies a person,
nothing free-text leaves the machine, and the off-path is covered by tests
(unasked/denied/env-off send zero events). Opt-in rates will be a fraction
of opt-out rates — that is the accepted trade. Adding a new event costs one
allowlist entry, a docs-table row, and tests; anything richer than
identifier-shaped properties is rejected by design.
