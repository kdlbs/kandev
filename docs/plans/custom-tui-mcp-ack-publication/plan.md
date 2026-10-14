---
created: 2026-10-09
status: implemented
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
legacy_specs: []
---

# Implementation Plan: Preserve current agents during MCP strategy saves

## Overview

A successful custom terminal agent MCP save can erase profiles received while the save was pending,
restore deleted rows, and undo another card's accepted strategy in the open Agents settings page.
One sequential work order owns the rendered regression and the smallest card-local correction.
ROOT reviewed this package and released implementation after the completed design checkpoint.

Agents owns configured definitions and profile membership. Extend its existing
[operator-registered requirements](../../specs/agents/requirements/custom-acp-agents.md) and
[design](../../specs/agents/system-design/custom-acp-agents.md#terminal-mcp-save-acknowledgement).
The prior criteria omit local ACK preservation; this package adds AC-AGENTS-CUSTOM-ACP-001.8 through
001.10. Existing custom-ACP and live-discovery packages are related history, not dependencies or reopened work.

## Confirmed cause and evidence

Inspected source: `7511d1ac53fb29a2f30cd3ebc6e38838d69a5198` on
`feature/preserve-agent-updat-330`. `custom-tui-mcp-card.tsx` captures `savedAgents` before awaiting
`updateCustomTUIAgentMCPStrategy`, then calls `setSettingsAgents` on that captured whole list. Even its
profile-preservation override uses captured `item.profiles`. A React rerender while pending does not
replace the already-running callback's closure.

Ordered trace: start strategy save for A; real profile-created event adds P to current A; ACK maps
old A and old profiles, so P disappears from the settings profile rows. Update/delete events are
similarly regressed. Two cards A/B can both capture the same list: accepting A then B restores A's
old strategy, and reversing completion reverses the loser. Existing setter replaces membership, so
new agents vanish and removed agents return. This is source-confirmed evidence; no rendered,
real-provider, browser, database, or product test has run in this design turn.

## Scope

### In scope

- Existing custom terminal card's successful ACK publication from current owning `AppStoreApi`.
- Accepted MCP strategy and coupled support fields only; preserve current config, metadata,
  profiles, membership, and sibling accepted changes.
- Permanent rendered actual-card/provider/store deferred-transport regression coverage.

### Out of scope

- Task98 executor-policy files; global catalogue fetching; backend, registry, API and WS redesign.
- Timestamps/version frameworks, same-agent cross-client strategy ordering, lifecycle polish.
- Task start, profile selection, discovery fetching, new copy/layout, public API changes.
- Design-turn production/permanent tests, installs, product checks, DB/browser/build, commit/PR,
  heavy commands, delegates, or new tasks/tabs/sessions.

## Technical approach

Use `useAppStoreApi()` in `CustomTUIMcpCard`. After awaiting the real settings API, read the current
list and synchronously use its existing action. Map existing membership; only the request target with
matching response identity and an eligible current TUI config gets `supports_mcp` and the accepted
`mcp_strategy`. Merge that key into current config; omitted response key means Off. No full response
spread, profile merge, missing-row insertion, or separate flattened-projection write.

`SetCustomTUIAgentMCPStrategy` changes strategy and support, but returns full `AgentDTO`, including
profiles, capability fields, and timestamps. Those extra fields are not this ACK's publication authority.
Keep the current saving/handled-error/toast path. No new store action or helper export is needed.

## ASCII UI preview

UI-01: Existing `/settings/agents` card, after save with a concurrently received profile. Both desktop
and phone use this composition; existing phone touch sizing and settings scroll owner continue.

```text
Before ACK                  After ACK (proposed)
Custom terminal A           Custom terminal A
  Profile: New profile        Profile: New profile
  MCP: Off [disabled]         MCP: claude [enabled]
Custom terminal B           Custom terminal B
  MCP: codex                  MCP: codex
```

Illustrative domain names, not new product copy. Current source predicts the defective ACK drops New
profile and can restore B's previous strategy. Required structure is the existing card/profile/control
order; this change only preserves received content. AC-001.8/001.9/001.10 are exercised by rendered
integration checks. No viewport-dependent state is added.

## Test inventory and regression matrix

Existing coverage:

- `app/settings/agents/custom-tui-mcp-mount.test.tsx`: real card/index wrapper and provider,
  eligibility, initial value, handled error; API-level stubs and static agent props miss this race.
- `components/settings/mcp-strategy-select.test.tsx`: real Off/key mapping; no async publication.
- `lib/ws/handlers/agents.test.ts` and `agents-settings-updated.test.ts`: live profile/settings
  application with narrower stores; useful message shapes, not substitute rendered-card evidence.
- `components/state-provider.test.tsx`: provider/store composition pattern. Profile catalogue
  publication fixtures illustrate transport barriers; do not import/modify task98 executor fixtures.
- Desktop/mobile `custom-tui-mcp-strategy-selector.spec.ts` files: real selector geometry/creation,
  not pending saved-agent acknowledgement coverage.

Primary permanent test owner: `components/settings/custom-tui-mcp-card.test.tsx`, with a nearby
test-helper only if necessary to stay within file limits. Real `StateProvider` constructs the actual
`createAppStore`; capture it using an unmocked `useAppStoreApi`. Subscribe props to current list,
render actual `CustomTUIMcpCard`, `MCPStrategySelect`, and `AgentProfilesSubList`, and use actual
Toast/Tooltip providers as needed. Mock only `fetchJson`, preserving real API normalization. Use
explicit admission and settlement promises, not sleeps or a mocked selector callback.

| Scenario | Evidence | Criteria |
| --- | --- | --- |
| `retains profile creation received while strategy save is pending` | Real created handler adds selected-agent profile; real profile link remains after ACK; RED loses it | 001.8 |
| Profile edit and deletion | Real updated/deleted handlers modify target/sibling profiles; ACK retains current rows/model and absence, flattened options unchanged | 001.8 |
| Mixed membership and metadata | Real setter adds agent, removes another, edits target config/capability and unrelated metadata; stale full ACK changes only owned fields | 001.8 |
| Two cards, A then B and B then A | Both PATCH requests pending before either completion; both accepted strategies remain rendered; each full response has old unrelated data | 001.9 |
| Saving target removed | Current target absent before late ACK; remains absent, no resurrection; settle captured transport even after unmount | 001.9 |
| Ordinary on/off control | Accepted response strategy/support appears without WS echo, control disabled while pending then usable; empty/omitted key disables injection | 001.8, 001.10 |
| Ordinary and handled rejection with current changes | No state rollback, ordinary error visible, handled error suppressed, surviving control usable | 001.10 |

Before resolving each response, assert the live update is already observable to prevent vacuous
preservation checks. For mixed cases assert exact current membership/fields and untouched profile
objects using independently specified expected values, not a copy of the merge algorithm. Target
removal can pass on old code only if old list was empty; seed the target and prove it existed when
the request began. Keep unique profile IDs per case and settle all requests during teardown.

## E2E tests and mobile parity

This is the mobile-parity skill's state-normalization exception: actual rendered component/store
integration covers the whole affected input/transport/live-event/publication path without changing
layout, touch, scrolling, navigation, or breakpoints. No new Playwright cases or build are required.
Existing selector E2E files remain geometry evidence only. Design-turn browser checks are forbidden.

Source confirms task-start choices consume `agentProfiles`, while this card only replaces
`settingsAgents`. Assert the real event-updated flattened state survives; do not claim that options or
launches disappeared, mount an unrelated picker, or broaden the repair to fetch ownership.

## Work orders

- [x] [Task 01: Publish accepted strategy into current agent state](task-01-publish-current-agent-strategy.md)

Sequential, no implementation delegates. Later release must precede all permanent test/production edits.

## Verification results

Design documentation validation passed on 2026-10-09:

- `python3 scripts/list-docs.py validate`: 368 decisions and 1496 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Scoped `git diff --check`: passed; status confirms two modified spec files and the new plan directory.
- Pure `.github/scripts/pr-docs.cjs` `validateCoverage`: actual docs-only package exempt with zero
  errors; proposed card/test paths covered by Task 01 with zero errors. This is preflight evidence,
  not a hosted check or proof of implementation.

The shell lacks `node` on PATH; preflight used the already installed
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`. Nothing was installed or built.
Task 01 now records the expected rendered RED, full affected GREEN (5 files/48 tests), final
changed regression rerun (17 tests), scoped zero-warning lint and successful ordinary typecheck. Do not promote the existing
broader draft spec pair based on this small repair alone.

## Implementation verification

Task 01 is done locally. The actual card now reads its current owning store after PATCH, checks
request/response/current eligible target identity, and publishes only accepted support and strategy.
Current nested config, profiles, membership, timestamps and capabilities remain intact. Permanent
coverage includes actual profile events, both sibling completion orders, removed/now-ineligible
rows, mismatched response identity, accepted-key canonicalization, empty/omitted Off and failures.
The explicit state-only mobile exception applies; no browser/build/DB/screenshots were requested.
Normal active commit hooks/publication and exact-current-head hosted evidence remain pending.

## Delivery and resources

Task `5c09b39f-daf0-4304-8326-cc68473e333a`, session `39f4155a-2f5a-4e59-a2ff-13064d7dfd74`.
At the design checkpoint no GLOBAL LOCAL-HEAVY lease was held. ROOT subsequently transferred the
exclusive lease from task98 to CHILD99 for implementation. One frozen dependency install and
single-worker targeted checks are authorized; foreign resources remain untouched.
Preserve all foreign worktrees/caches/paused resources. At design completion stop for later explicit
ROOT implementation interrupt; no approval/model prompt. Kandev task plan preserves the system marker,
question barrier, title ownership, autopilot scope, user edits, and final delivery constraints.

After reviewed release, delivery is already authorized: targeted RED/GREEN and normal active hooks;
freeze/publish candidate; record actual local joins/current known-own absence, return lease, then
direct hosted work in the same turn. One 90-minute observer; inspect configured App347564 full
exact-head/all-files/source=covered-head substantive review while CI runs before any necessary
request. Batch substantive blockers and freeze heads; defer only grounded optional nonblocking
polish. Callback failures never gate progress. ROOT grants serial merge only after required
six+product parents actually SUCCESS and owns independent merge/FF/archive/refill. No compulsory
END/WFI or historical hash/PID sweeps. Stop only at design checkpoint or an actual bounded blocker.

## Risks and documentation impact

- Same-agent writes across clients can still race on the strategy field; explicitly excluded.
- Spreading full response config/metadata would reintroduce a narrower stale overwrite; pin owned fields.
- A static agent prop or mocked publication could make the test pass without exercising reactivity.
- Public docs describe current operator actions, not ACK replacement; this repair restores that flow
  with no new controls/contracts. Internal docs only. ADR 0014 remains authoritative for injection;
  no new ADR because this is a card-local repair within existing boundaries.
