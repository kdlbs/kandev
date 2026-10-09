---
id: "01-publish-current-agent-strategy"
title: "Publish accepted MCP strategy into current agent state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
acceptance_criteria:
  - AC-AGENTS-CUSTOM-ACP-001.8
  - AC-AGENTS-CUSTOM-ACP-001.9
  - AC-AGENTS-CUSTOM-ACP-001.10
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
---

# Task 01: Publish accepted MCP strategy into current agent state

## Summary

Prove the pending strategy ACK overwrites current settings using the rendered real card/provider/store.
Correct only that card's publication to preserve current rows and profiles while applying accepted
MCP strategy/support. Follow the [plan's regression matrix](plan.md#test-inventory-and-regression-matrix).

## In scope

- Add permanent deferred-transport tests with real settings API, StateProvider/createAppStore,
  subscribed agent props, real selector/profile rows, real actions and registered profile WS handlers.
- Apply accepted strategy/support to the current existing target through the owning store API.
- Record actual RED/GREEN and validation results and synchronize this work order and manifest.

## Out of scope

Task98 executor-policy files/fixtures, global catalogue fetches, backend/registry/API/WS changes,
timestamp/version frameworks, same-agent cross-client ordering, lifecycle polish, new UI/copy,
task-start behavior, broad suites, extra local QA/review, delegates or new platform tasks/sessions.
Implementation was released after the completed design checkpoint.

## Acceptance

1. The named creation regression fails on current production due to loss of a real received profile
   after deferred ACK, then passes after correction. No mocked store, provider, publication, selector,
   settings API wrapper, or isolated merge helper is used. All matrix cases assert actual current state.
2. Profile create/edit/delete, mixed agent membership/current config and metadata, and both sibling
   completion orders survive ACK. Removing the saving target before settlement never restores it.
   Separate event-updated `agentProfiles` remains intact. Only accepted strategy/support changes.
3. Ordinary on/off saves reflect response fields without WS echo; surviving control disables/settles
   correctly. Ordinary/handled failures preserve current changes and existing error behavior. Exact
   targeted checks pass; record resource state and stop/advance under ROOT's release constraints.

## ASCII UI preview

UI-01: Existing Agents settings card after concurrent profile creation, matching
[the complete plan preview](plan.md#ascii-ui-preview). Desktop and phone share the composition.

```text
Custom terminal A
  Profile: New profile   [retained after ACK]
  MCP: claude            [enabled after ACK]
Custom terminal B
  MCP: codex             [independent ACK retained]
```

Existing control order, localized labels, phone touch sizing, and scroll owner remain. Names/spacing
are illustrative. Criteria 001.8/001.9/001.10 map to the rendered regression matrix; no geometry edits.

## Verification

ROOT released implementation and the exclusive GLOBAL LOCAL-HEAVY lease on 2026-10-09 after
reviewing all four artifacts. Run shells with login disabled and prepend the existing
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin` to PATH. Use pinned pnpm 9.15.9 through Corepack; the temporary `/tmp/kandev-child99-bin/pnpm`
wrapper precedes Node on PATH so normal hooks use the same pin;
dependencies are absent, so one frozen apps install is required. Every Vitest command below adds
`--maxWorkers=1` and runs with `NODE_OPTIONS=--max-old-space-size=4096`. Scoped eslint adds
`--max-warnings=0`, including any actual helper file. Run ordinary web `pnpm run typecheck` once
before publication (or its equivalent normal active hook). All normal hooks/i18n remain active.
Only one heavy command runs at a time; retain and join every original command handle.

Only after later explicit ROOT implementation release and acquisition of the required heavy lease.
If workspace dependencies are absent, run `(cd apps && pnpm install --frozen-lockfile)` once under
that release/lease before product commands; do not install in this design turn.

First write only the primary creation regression and run RED before production edits:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run components/settings/custom-tui-mcp-card.test.tsx --maxWorkers=1 -t 'retains profile creation received while strategy save is pending')
```

Assert an expected behavioral failure, not import/setup failure. Add the remaining matrix, make the
minimum correction, then run all owning new tests plus the directly relevant existing controls:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run components/settings/custom-tui-mcp-card.test.tsx app/settings/agents/custom-tui-mcp-mount.test.tsx components/settings/mcp-strategy-select.test.tsx lib/ws/handlers/agents.test.ts lib/ws/handlers/agents-settings-updated.test.ts --maxWorkers=1)
(cd apps/web && pnpm exec eslint --max-warnings=0 components/settings/custom-tui-mcp-card.tsx components/settings/custom-tui-mcp-card.test.tsx components/settings/custom-tui-mcp-card.test-helpers.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a test helper is added, include its exact path in the eslint command. Normal active commit hooks
remain required in later authorized delivery; do not disable them. The one ROOT-required ordinary web typecheck is included; no build/browser
or full-suite gate is added to this bounded state repair. Run the pure local PR docs coverage preflight
below from repo root, then rerun affected tests only after new edits or a concrete failure warrants it.
It consumes raw Git bytes through Node, avoiding non-byte-preserving display helpers. It includes
untracked planned files and resolves referenced docs from the filesystem without hosted status writes.

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0');
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0');
const paths = [...new Set([...tracked, ...untracked].filter(Boolean))];
const docs = [
  'docs/plans/custom-tui-mcp-ack-publication/plan.md',
  'docs/plans/custom-tui-mcp-ack-publication/task-01-publish-current-agent-strategy.md',
  'docs/specs/agents/requirements/custom-acp-agents.md',
  'docs/specs/agents/system-design/custom-acp-agents.md',
];
const fileContents = Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({
  changedFiles: paths.map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' })),
  fileContents,
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors, workOrders: result.workOrders }));
if (!result.ok) process.exitCode = 1;
NODE
```

This is the existing installed Node 24 path for this workspace; if it changes, use the current
already-installed Node 24 executable and record the exact substitution. No install is authorized
merely to run this documentation preflight.

## Files likely touched

- `apps/web/components/settings/custom-tui-mcp-card.tsx`
- `apps/web/components/settings/custom-tui-mcp-card.test.tsx` (new)
- `apps/web/components/settings/custom-tui-mcp-card.test-helpers.tsx` (only if needed)
- The existing requirement/design pair and this plan/work order for delivery results.

Read-only inputs: StateProvider/store, settings API, agent handlers/types, MCP select, profile sublist,
existing mount/handler tests. Do not change other agents' work, paused resources, or task98 files.

## Dependencies

None. Implementation requires later ROOT release and its resource grant; no callbacks are prerequisites.

## Risks

Use the response's accepted fields rather than requested strategy or the entire full response. Empty
strategy is Off. Keep a current eligible row check; never insert from a late response. Use per-case
unique profile IDs and settle all deferred requests even if the target card unmounts. Keep actual
providers/normalization and assert pre-ACK updates, so failure proof cannot be satisfied by a fixture.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/custom-acp-agents.md): AC-001.8 through 001.10.
- [Design](../../specs/agents/system-design/custom-acp-agents.md#terminal-mcp-save-acknowledgement).
- [Plan](plan.md): source trace, exact field ownership, existing tests, regression matrix, resources,
  and later hosted delivery/ROOT merge gates.
- Local root/web AGENTS, fix/spec/plan/TDD/mobile-parity skills. No new ADR is needed.

## Results

Completed local implementation on 2026-10-09:

- One `(cd apps && pnpm install --frozen-lockfile)` through pinned Corepack 9.15.9: pass; dependencies
  were absent. Original install command joined. No lockfile change.
- Primary deferred rendered RED, with `--maxWorkers=1` and 4096 MiB Node cap: expected behavioral
  failure, received profile link became null after ACK. Actual StateProvider/createAppStore, card,
  profile rows, selector, API normalization and profile-created handler ran. Only transport was mocked;
  this does not claim a real external provider session or browser/server run.
- Full affected GREEN command above: 5 files, 48 tests passed, including 17 new regression cases.
- After lint-required suite grouping and fixture type corrections, only the changed regression file
  reran with the same worker/memory caps: 1 file, 17 tests passed. Unchanged passing controls were
  not replayed. Production correction did not change after the full affected GREEN.
- Scoped eslint including the actual helper, `--max-warnings=0`: pass. Initial callback-size warning
  was resolved by grouping tests without disabling the rule.
- Ordinary `pnpm run typecheck`: pass after correcting the new fixtures' HTTP/WS payload shape,
  store-state fields, literal sibling IDs and omitted-key construction. The initial failed run was
  joined before correction and rerun. No build, browser or database check ran.
- Catalogue validation: 368 decisions and 1496 specifications passed. All-spec lint and whitespace
  gate passed. Pure PR docs coverage preflight: covered, zero errors. Its initial overbroad document
  load exceeded the byte budget; corrected to the exact four referenced artifacts and reran.

Normal active commit hooks and publication remain the next delivery step. Current HEAD check/review
status is external pending; ROOT retains the independent serial merge gate. The broader existing
custom-ACP draft spec pair is not promoted by this bounded repair.
