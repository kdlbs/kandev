# ADR-2026-10-01-agent-runtime-update-authority: Scope Automatic Updates to Verified Runtime Activation

**Status:** accepted
**Date:** 2026-10-01
**Area:** backend, frontend, protocol

## Context

Issue #4129 expands background awareness and opt-in automatic updates to every registered agent. Existing managed updates validate exact npm candidates before persisting selection. Native binaries and separately installed CLIs have different owners; PATH discovery does not establish npm ownership. Vendor self-update commands can mutate a live installation without a validated isolated candidate or reliable restoration.

## Decision

Use one agent-neutral runtime capability projection for managed, manual, and unsupported cases. Install-wide automatic consent binds to the trusted runtime identity and is off by default. Automatic updates use the same verified candidate and activation boundary as manual managed updates; they never infer installation ownership from a binary name or run a guessed vendor command. External/native runtimes without verified safe staging and restoration expose read-only release status when available and vendor guidance. Running sessions and executor-owned installations are never mutated by the updater. Manual version selection disables automation, and opt-out is checked at activation.

Reuse the notification service's existing update preference and durable per-recipient delivery claims for runtime availability and update outcomes. Source cache and scheduling are shared across agents.

## Consequences

All agents are visible without promising an updater Kandev cannot safely execute. Managed packages, including the Codex native protocol package, can opt into validated automatic activation. Vendor self-update and separately installed dependencies remain explicit manual capabilities. Adding a supported native updater later requires verified owner, candidate isolation, validation, activation, restoration, and live-session contracts.

## Alternatives Considered

- Automatic global npm installs for any discovered binary were rejected because PATH does not prove package ownership and failure can replace the known-good runtime before validation.
- A vendor-specific scheduler per agent was rejected because it duplicates caching, consent, and notification policy.
- Claude-only or ACP-bridge-only automation was rejected because the registry already includes other managed packages and native runtime mechanisms.
- Default-on automation was rejected because users must authorize runtime behavior changes independently of background awareness.
