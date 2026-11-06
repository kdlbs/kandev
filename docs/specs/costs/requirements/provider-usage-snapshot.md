---
status: active
system: costs
created: 2026-10-15
owners:
  - kandev
---

# Provider usage snapshots

The costs system owns provider quota meaning and provenance. Agent settings
exposes these readings independently of Office and recorded task costs.

### REQ-COSTS-PROVIDER-USAGE-SNAPSHOT-001: Account-aware provider readings

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.1:** When enabled, the ordinary agent-profile settings list shall show the provider, shared account identity, model billing class, reported utilization windows, reset instants and reading time for supported local credential bindings. Profiles sharing that credential shall share one cached reading.
- **AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.2:** Missing credentials, unsupported bindings, unavailable reads and accounts without a usage API shall remain distinguishable from measured zero. Free OpenCode models shall not inherit the paid Go subscription windows.
- **AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.3:** Desktop and phone shall expose the same source and failure state with an accessible explicit refresh control. Reads shall not start model executions or recurring polling.
- **AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.4:** An independent native feature toggle shall hide the added UI and reject its API before fetching when disabled. Existing Office utilization, account holds, profile order, settings and task/session history shall retain their behavior.
- **AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.5:** Public readings shall not expose credentials, secret references, credential paths or raw provider failures. An uncertain execution credential binding shall remain unknown instead of borrowing another account's usage.

## Scope

This increment adds OpenCode Go provider readings and reuses the existing
Claude, Codex and TeamClaude clients. OpenCode Zen/free bindings explicitly
report no usage API. Other providers and profile-specific credential overrides
remain unsupported in this increment.

The snapshot read surface does not itself enforce limits. The shared Kandev-turn
increment below adds its own admission and settlement boundary. Provider-request,
token/USD and model/group budgets, learned values, automatic routing, local cost
statistics, cache backoff changes and operating installation remain separate work.

## Implementation plans

### REQ-COSTS-PROVIDER-TURN-ALLOWANCE-002: Shared native turn admission

#### Acceptance criteria

- **AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.1:** Enabled native settings APIs shall configure optional shared-credential maximum concurrent Kandev turns, turns per fixed reset window, and an explicit block-until instant. Omitted owner fields and stored learned/unknown fields shall survive partial saves. A Kandev turn is not a provider HTTP request or a prepaid token/USD amount.
- **AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.2:** Immediately before the common agentctl prompt dispatch, configured allowances shall atomically reserve one durable turn across profiles sharing the verified account. Existing submission/generation identity shall make retransmission and an in-flight steer idempotent.
- **AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.3:** Confirmed unsent failures shall return the reservation; a validated completion or confirmed runtime stop shall return concurrency while retaining the admitted turn in its period. Ambiguous delivery shall retain the claim. A restart shall not silently erase claims.
- **AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.4:** A host-default observation shall be used only after verifying actual local executor, profile and effective credential environment. An unresolved binding with potentially applicable configured limits shall reject dispatch; a verified disjoint native provider family shall not inherit another provider's limitation. Free models shall not consume paid provider windows. Unknown provider usage shall remain unknown.
- **AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.5:** The existing native flag shall close added settings and dispatch enforcement when off while retaining stored settings/claims. Existing queue, grade admission, generation, cancellation, task/session history and model order shall remain owned by their current consumers.

This first allowance increment does not complete numeric provider-request,
token/USD, model/group budgets, natural-error learning adapters, actual remote
credential binding or the reset/learned-value editor. Those remain original work.

- [Provider usage snapshot](../../../plans/provider-usage-snapshot/plan.md)
