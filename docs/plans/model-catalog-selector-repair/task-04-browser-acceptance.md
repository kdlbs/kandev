---
id: "04-browser-acceptance"
title: "Prove desktop and phone owning flows"
status: pending
wave: 4
depends_on:
  - "03-contextual-acquisition"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.5
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 04: Prove desktop and phone owning flows

## Summary

Prove the repaired shared selector through profile, workflow, and live ownership on desktop and phone.
Use deterministic provider-shaped subprocesses and explicit persistence evidence.
Complete only the corresponding original Task 07 gaps, with actual results and public guidance.

## In scope

- Extend the disposable Devin descriptor and mock peer with native grouped catalogs and `models list --format json` dispatch.
- Give the mock Devin launch a qualified `acp` command shape so production command preparation runs unchanged.
- Exercise the same resolved fake executable for ACP and the native listing, with independent Thinking/Speed snapshots.
- Reuse Task 02's sanitized observed shape with public fixture labels and separate synthetic rejection cases.
- Add the four grouped-catalog Playwright files and shared helpers already identified in the original Task 07.
- Cover the plan's complete E2E matrix, including first-profile discovery without a saved parent.
- Add a separately labelled neutral second-provider presentation fixture with different dimension names and opaque values.
- Keep successful Devin discovery, resolution, Save, and live application on real backend paths.
- Update public model-selection guidance and synchronize original/new delivery results without rewriting historical counts.

## Out of scope

- Commercial inference, credentials in CI, successful HTTP probe fulfillment, or browser fixtures presented as native qualification.
- New Devin permission capability scenarios or unrelated screenshots and documentation rewrites.
- A generic QA, review, simplify, or full verification phase.

## Acceptance

1. Both projects prove singleton/search and verified Fusion configuration through profile Save/reload, workflow Save/reload, and session Apply with unchanged conversation identity.
2. Invalid combinations, stale results, missing options, source fallback, cancellation, viewport changes, and partial errors preserve the correct draft or confirmed state.
3. Phone geometry/interaction, public documentation, and linked plan statuses contain actual evidence. No remaining qualification gap is hidden by a mock pass.

## ASCII UI preview

Use all [UI-5053 views](plan.md#ascii-ui-preview). Keep the same labels when recording rendered comparisons.

```text
UI-5053-01, desktop catalog
[ Search ]
Ordinary singleton model
Variant family
  Variant A
  Variant B
Fusion                            >
Unknown pairing

UI-5053-02, desktop configuration
< Models                     Fusion
Lead       [ choice               > ]
Sidekick   [ choice               > ]
Thinking   [ returned value       > ]
Speed      [ returned value       > ]
                [ Use configuration ]

UI-5053-03, phone sheet
         Model settings         [x]
< Models                     Fusion
[ Lead                            > ]
[ Sidekick                        > ]
[ Thinking                        > ]
[ Speed                           > ]
[         Use configuration         ]
             safe area
```

The phone replaces the sheet body for dimension choices and returns through Back.
It does not stack popovers. One body owns vertical scrolling and Apply stays reachable.
Measure 44px phone/coarse-pointer hit targets, safe-area clearance, containment, and zero horizontal document overflow.
At 767px and 768px, check the actual responsive composition and retain the unsaved draft.
Desktop controls keep normal density. Provider names are data, and Kandev copy uses translations.
Map the views to AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5, 003.1 through 003.4, and 004.1 through 004.5.

## Fixture and scenario details

Keep the normal `mock-agent` seed intact and use owned disposable Devin rows.
The mock peer must dispatch both `acp` and native model listing before its existing ACP server starts.
Only the gated E2E Devin registration uses that command shape.
Restore any worker environment, files, profiles, workflow changes, and source scenario in `finally`, including error paths.

The real Devin-shaped fixture proves acquisition and backend normalization.
The neutral second-provider fixture proves generic presentation through the existing test-only app-store bridge.
Label that distinction in results. It cannot replace the real subprocess-shaped fixture or Task 02 capture.

Cover these outcomes in both viewport projects:

- A singleton beside a multi-choice group, nested/repeated labels, provider descriptions, and path search.
- One Fusion entry, Lead-family labels, filtered Sidekicks, and exact returned Thinking/Speed options.
- An invalidated Sidekick requires explicit replacement before Apply.
- Opening, Back, and dismissal never mutate the owning model. Re-entry and resize retain the unfinished draft.
- Profile and workflow Apply affect only the draft until Save. Reload reads exact persisted model/options.
- A first Devin draft can discover/configure before parent/profile creation.
- Live Apply preserves task/session/native conversation identity and profile defaults.
- A successful model mutation followed by an option error shows confirmed partial state and retries only pending options.
- A delayed model-A resolution cannot replace model B's controls. Removed Speed is absent after a complete matching snapshot.
- Unknown pairing and native source failure leave ordinary exact choices available.
- Reload/restore and historical turn labels use the existing attribution owner.

Arm HTTP/WS evidence before each initiating action and wait on its causal result.
Use the guarded runner and assert unique active picker instances before interaction.
Compare the rendered phone structure with UI-5053-03 and record any unresolved difference.

## Verification

Run from the repository root after the workspace dependency install.
Keep managed E2E commands sequential. Each command rebuilds the required runtime and web assets.
All four new browser files must exist before these commands run.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent ./internal/agent/registry -count=1)
(cd apps/web && pnpm exec vitest run components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx components/settings/profile-model-fields.test.tsx components/settings/workflow-session-config-editor.test.tsx lib/apply-model-configuration.test.ts)
(cd apps/web && pnpm e2e:run --shards 1 --project chromium tests/settings/grouped-profile-workflow-models.spec.ts tests/chat/grouped-model-selector.spec.ts tests/chat/model-selector-consecutive-switch.spec.ts tests/settings/first-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --shards 1 --project mobile-chrome tests/settings/mobile-grouped-profile-workflow-models.spec.ts tests/chat/mobile-grouped-model-selector.spec.ts tests/chat/mobile-model-selector.spec.ts tests/settings/mobile-first-profile-capability-discovery.spec.ts tests/settings/mobile-agent-profile-config-selector.spec.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings e2e/helpers/grouped-model-catalog.ts e2e/tests/settings/grouped-profile-workflow-models.spec.ts e2e/tests/settings/mobile-grouped-profile-workflow-models.spec.ts e2e/tests/chat/grouped-model-selector.spec.ts e2e/tests/chat/mobile-grouped-model-selector.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git ls-files --modified --others --exclude-standard -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/model-catalog-selector-repair docs/plans/first-draft-profile-discovery docs/specs/agents docs/public
git status --short -- docs/plans/model-catalog-selector-repair docs/plans/first-draft-profile-discovery
```

Add every further changed frontend helper/spec to the focused lint command.
Record the runner's actual discovered/pass counts and each command's result.
Public docs change only after the implemented behavior passes.
The original qualification work order closes only after Tasks 02 and 03 pass.
The original Task 07 closes only after its complete remaining acceptance matrix passes.

Run this documentation reference preflight from the same repository-root shell.
The two runtime paths deliberately force coverage validation, even in a design-only diff.
This check proves reference integrity, not runtime behavior.

```bash
node <<'NODE'
const fs = require('node:fs');
const path = require('node:path').posix;
const { execFileSync } = require('node:child_process');
const docs = require('./.github/scripts/pr-docs.cjs');
const names = (args) => execFileSync('git', args, { encoding: 'utf8' }).split('\0').filter(Boolean);
// Compare committed work-order changes against the PR base like CI does, and keep
// untracked files. Comparing with HEAD would omit committed work orders.
const baseRef = process.env.KANDEV_DOCS_BASE_REF || 'origin/main';
const base = execFileSync('git', ['merge-base', baseRef, 'HEAD'], { encoding: 'utf8' }).trim();
const changed = [...new Set([...names(['diff', '--name-only', '-z', base]), ...names(['ls-files', '--others', '--exclude-standard', '-z'])])];
const contents = {};
const ids = new Set();
for (const file of docs.selectChangedWorkOrders(changed)) {
  contents[file] = fs.readFileSync(file, 'utf8');
  const { data } = docs.parseFrontmatter(contents[file]);
  for (const id of data.requirements) ids.add(id);
  for (const ref of [data.plan, ...data.system_design]) {
    const target = path.normalize(path.join(path.dirname(file), ref));
    contents[target] = fs.readFileSync(target, 'utf8');
  }
}
for (const file of names(['ls-files', '-z', '--', 'docs/specs/**/requirements/*.md'])) {
  const body = fs.readFileSync(file, 'utf8');
  if ([...ids].some((id) => new RegExp('^#{1,6}\\s+' + id + '(?:\\s|:|$)', 'm').test(body))) contents[file] = body;
}
const result = docs.validateCoverage({ changedFiles: [...changed, 'apps/web/components/model-catalog-groups.tsx', 'apps/backend/internal/agentctl/acpcompat/devin_catalog.go'], fileContents: contents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exit(1);
NODE
```

## Files likely touched

- New `apps/backend/cmd/mock-agent/devin_catalog.go` and focused dispatch/config tests
- `apps/backend/cmd/mock-agent/main.go` and existing host-CLI dispatch helper
- `apps/backend/internal/agent/registry/provider.go` and `provider_test.go`, only for gated mock command wiring
- New `apps/web/e2e/helpers/grouped-model-catalog.ts`
- New `apps/web/e2e/tests/settings/grouped-profile-workflow-models.spec.ts` and `mobile-grouped-profile-workflow-models.spec.ts`
- New `apps/web/e2e/tests/chat/grouped-model-selector.spec.ts` and `mobile-grouped-model-selector.spec.ts`
- Existing first-profile, session store, and model-selector helpers only where required
- `docs/public/agents-and-profiles.md`, model groups and composed selections
- Results/status in this package and the linked original Tasks 04/07 and qualification record

## Dependencies

Tasks 01 through 03, including actual captured source qualification.

## Risks

- Successful HTTP fulfillment or neutral browser seeding can hide a broken native acquisition path.
- The disposable descriptor currently uses a generic mock command. Its native subcommand shape must be tested explicitly.
- Short phone content cannot prove scrolling or safe-area clearance.
- Discovery, Apply, Save, and restored state require separate evidence to avoid a false persistence pass.

## Parallelism

`sequential`

## Inputs

- [Plan and E2E matrix](plan.md#e2e-tests)
- [Shared selector and mobile design](../../specs/agents/system-design/grouped-model-catalog.md#shared-selector)
- [Original Task 07](../first-draft-profile-discovery/task-07-combined-acceptance.md)
- Existing `devin-profile-permissions.ts`, first-profile discovery, model-switch, and mobile selector tests
- `apps/backend/cmd/mock-agent/AGENTS.md`, `/e2e`, and `/mobile-parity`

## Results

Blocked on Task 03.
The Task 01 component evidence covers the singleton projection on both responsive branches, but the grouped/Fusion browser matrix and public guidance require the qualified acquisition path first.
No mock-agent native dispatch or Playwright file was added, because doing so would present an unqualified shape as captured evidence.
