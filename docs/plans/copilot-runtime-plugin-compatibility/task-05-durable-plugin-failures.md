---
id: "05-durable-plugin-failures"
title: "Preserve actionable plugin startup failures"
status: done
wave: 4
depends_on:
  - "04-contract-admission"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-002.3
  - AC-AGENTS-COPILOT-PLUGINS-004.8
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.13
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-006.11
  - AC-AGENTS-COPILOT-PLUGINS-006.12
  - AC-AGENTS-COPILOT-PLUGINS-007.4
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-plugin-contract-compatibility.md
---

# Task 05: Preserve actionable plugin startup failures

## Summary

Carry a typed, safe plugin-preparation cause through bootstrap wrapping into
durable task/session feedback. Show the actionable reason after reload on
desktop and phone without exposing native state or losing selection intent.

## In scope

- Recognize `copilotplugins.Error` before generic `bootstrapFailureFor`
  classification. Preserve bounded reason and exact runtime as structured evidence.
- Add `plugin_preparation_failed` to existing cause normalizers, with safe
  reason/version fields and constant recovery text. Do not forward raw `Error()` strings.
- Trace ordinary and MCP-created starts through the real shared failure
  callback, durable `last_agent_error`, status summary, transcript, and HTTP/WS consumers.
- Extend existing recovery-card normalization and localized presentation.
  Preserve attempt stamps, ownership, unrelated classifications, and current retry controls.
- Add privacy, stale-attempt, reload, and desktop/phone regressions.
  Update public failure recovery guidance with the implementation.

## Out of scope

Another error store, new retry or fallback policies, plugin installation,
runtime switching, raw log disclosure, and changes to retained processes.

## Acceptance

1. `TestCopilotPluginBootstrapFailurePreservesReason` exposes the current
   generic-detail loss before correction, then retains the safe typed cause.
2. `TestCopilotPluginStartupFailureProjection` proves durable/UI propagation
   for ordinary and MCP-created starts, reload, attempt fencing, and private-data exclusion.
3. Desktop and phone recovery cards show localized cause and repair guidance.
   Foreign providers, authentication, model, quota, npm, and unknown failures retain their semantics.

## ASCII UI preview

UI-04, [full scope](plan.md#confirmed-direction-change-2026-11-05):

```text
Task conversation (desktop and phone)
Copilot could not prepare the selected plugins.
Reason: plugin inventory format is incompatible.
Runtime: <exact selected version>
Repair plugin state or review profile selections, then retry.
[Existing recovery actions]
```

Use the existing card in the conversation scroller. Phone text wraps and
actions remain at least 44px with existing safe-area clearance.
Show no plugin identity, host path, raw command, or native output.
Copy is illustrative and all final text is localized.

## Verification

Run commands from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestCopilotPlugin|TestStartModel)' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/models -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestCopilotPluginStartupFailure' -count=1)
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/copilot-plugin-startup-failure.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-copilot-plugin-startup-failure.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The two E2E files and named Go regressions are proposed outputs. Seed failures
through disposable runtime fixtures, never an operator instance. Record actual
commands/counts, privacy controls, and final traceability preflight. Run
changed-Go lint and focused Oxlint against the complete changed-file inventory.

The first desktop invocation builds the backend, Vite, and fixture assets.
Later browser runs may use the managed runner's documented `--no-build` option
when those assets are unchanged. Record the build provenance and exact final
commands; a test-only assertion or environment-fixture edit does not require
another production build.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/arguments.go` and preparation context/error tests.
- `apps/backend/internal/agent/runtime/lifecycle/{bootstrap_failure,copilot_plugins}.go` and new `copilot_plugin_bootstrap_failure_test.go`.
- `apps/backend/internal/task/models/launch_errors.go` and normalization tests.
- `apps/backend/internal/orchestrator/event_handlers_agent.go` and new `copilot_plugin_startup_failure_test.go`.
- `apps/web/lib/types/task-status-summary.ts`, `apps/web/lib/session-last-agent-error.ts`, and its tests.
- Existing `apps/web/components/task/chat/session-bootstrap-recovery-{model,content,card}` modules and tests.
- New `apps/web/e2e/tests/session/copilot-plugin-startup-failure.spec.ts` and `mobile-copilot-plugin-startup-failure.spec.ts`.
- All shipped `task.json` locale catalogs and `docs/public/agents-and-profiles.md`.

## Dependencies

Task 04 supplies the current capability/schema reasons. Reuse its error model
and the existing bootstrap/error storage path rather than creating a second projection.

## Risks

Generic startup sanitization is intentional. Extend only typed, validated
provider evidence; a copied backend error string can expose private data.
Late failures must not overwrite a newer execution's recovery state.

## Parallelism

`sequential`

## Inputs

The new contract-admission design, `bootstrapFailureFor`, `BootstrapFailure.SafeDetail`,
`handleAgentStartFailed`, and existing agent-error cause/recovery-card conventions.

## Results

Implemented and product-validated with the authorized native worker. Runtime
metadata confirmed `gpt-6-luna` with effort `max`. The coordinator owns final
artifact and workflow closure. The worker performed no commit or push.

### Causal Red and final Green

Before production edits, this compiled regression failed with
`safe code = "unknown", want "plugin_preparation_failed"`:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestCopilotPluginBootstrapFailurePreservesReason$' -count=1)
```

The following final Go commands all passed:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestCopilotPlugin|TestStartModel)' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/models -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestCopilotPluginStartupFailure' -count=1)
```

The lifecycle tests preserve the captured runtime through real manual-mount
conflicts and reject foreign providers, unknown reasons, and invalid versions.
Model tests cover durable JSON decode/round-trip and clear runtime evidence
from non-plugin causes, including valid model and permission-mode reasons.
Runtime evidence uses the existing stable SemVer parser, including build metadata.

The orchestrator regression has four cases: an ordinary explicit start, the
MCP-equivalent direct `IntentStart`, the MCP explicit-workflow
`IntentPrepare`/`DeferredStart` then `IntentStartCreated`/`InitialCreatePrompt`
sequence, and a superseded ordinary attempt. It uses the real Executor
start-failure callback, SQL failure commit, status projector, and transcript.
It checks reload, summary storage and events, privacy, and stale ownership.
The test supplies the handler's actual request shapes; it does not invoke
the MCP handler itself.

The initial fake launch omitted its required `executors_running` row, so the
SQL ownership fence correctly refused the failure transition. Correcting
the fixture made the real callback path pass without changing that fence.
Coordinator feedback also corrected the initial MCP `AutoStart=true` shape
and made stale assertions wait for completed failure handling.

### Frontend, localization, and public guidance

All commands below passed on final production source. Vitest passed 50 tests
across three files. Oxlint-driven helper extractions were followed by another
Vitest run, typecheck, i18n check/ratchet, and a fresh browser build.

```bash
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Public-doc regression tests passed 63/63; validation accepted 55 pages.
Backend and browser normalizers retain only the safe plugin detail, known
reasons, and validated runtime. Raw native diagnostics are not recovery copy.
The existing card renders localized reason, runtime, and repair guidance.

Final scope reconciliation restored generator-only rewrites in the unrelated
`zh-hk/automations.json` and `zh-tw/automations.json` catalogs to HEAD.
The worker confirmed provenance before restoring only those paths. Final
`i18n:check`, `i18n:ratchet`, and `git diff --check` passed. Copilot code and
catalog contents were unchanged by that cleanup.

### Final browser evidence

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/session/copilot-plugin-startup-failure.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-copilot-plugin-startup-failure.spec.ts)
```

Desktop passed 1/1 after rebuilding backend, Vite pseudo-locale, and fixture
assets from the final refactored source. Phone passed 1/1 sequentially on
those exact assets. These results cover the final Copilot code and catalogs.

The disposable backend keeps the E2E profile and test routes, with only
`KANDEV_MOCK_AGENT=false` selecting the real Copilot command builder.
A temporary PATH-first fake `npx` advertises `--plugin-dir`, then emits
malformed inventory containing synthetic privacy sentinels. The tests verify
real preparation, durable `invalid_inventory`, exact runtime `1.0.88`,
saved selections, reason/runtime/repair copy after reload, and no private
output in metadata, messages, or the card. Phone also checks the 44px recovery
action and horizontal containment. Cleanup restores the backend environment.
No external Copilot credentials or native network service were used.

The first browser fixture used MockAgent's `mock-agent` command and correctly
failed context validation. Switching only the provider override fixed the
fixture. Assertions then separated reason, runtime, and repair sentences
instead of assuming adjacency. These initial failures are not final evidence.

### Complete lint inventories

Changed-Go lint passed all 34 tracked/new Go files with zero issues.
Focused Oxlint passed all 23 changed/new web source, test, and E2E files
with warnings denied. Resolved input sets:

```bash
bash scripts/lint-go-changed \
  apps/backend/internal/agent/copilotplugins/arguments.go \
  apps/backend/internal/agent/copilotplugins/compatibility.go \
  apps/backend/internal/agent/copilotplugins/compatibility_test.go \
  apps/backend/internal/agent/copilotplugins/context.go \
  apps/backend/internal/agent/copilotplugins/copilotplugins_test.go \
  apps/backend/internal/agent/copilotplugins/inventory.go \
  apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_contract_test.go \
  apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_test.go \
  apps/backend/internal/agent/copilotplugins/runner.go \
  apps/backend/internal/agent/copilotplugins/runtime_compatibility_test.go \
  apps/backend/internal/agent/hostutility/copilot_plugins_test.go \
  apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugin_bootstrap_failure.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugin_bootstrap_failure_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_initial_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_process_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_process_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_cache_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_concurrency_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_review_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_test.go \
  apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility.go \
  apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility_test.go \
  apps/backend/internal/agent/settings/dto/copilot_plugin_discovery.go \
  apps/backend/internal/agent/settings/dto/dto.go \
  apps/backend/internal/agent/settings/handlers/copilot_plugin_discovery_test.go \
  apps/backend/internal/agent/settings/handlers/runtime_copilot_compatibility_test.go \
  apps/backend/internal/orchestrator/copilot_plugin_startup_failure_test.go \
  apps/backend/internal/task/models/launch_errors.go \
  apps/backend/internal/task/models/launch_errors_test.go \
  apps/backend/internal/task/models/models.go
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/settings/agent-runtime-update-copilot-compatibility.test.tsx \
  components/settings/copilot-plugin-profile-selection.test.tsx \
  components/settings/copilot-plugin-profile-selection.tsx \
  components/settings/copilot-runtime-compatibility.tsx \
  components/settings/use-agent-update-dialog-state.test.ts \
  components/task/chat/session-bootstrap-recovery-card.test.tsx \
  components/task/chat/session-bootstrap-recovery-content.tsx \
  components/task/chat/session-bootstrap-recovery-model.test.ts \
  components/task/chat/session-bootstrap-recovery-model.ts \
  e2e/helpers/copilot-plugin-fixture.ts \
  e2e/helpers/copilot-plugin-startup-failure.ts \
  e2e/tests/session/copilot-plugin-startup-failure.spec.ts \
  e2e/tests/session/mobile-copilot-plugin-startup-failure.spec.ts \
  e2e/tests/settings/agent-runtime-update-copilot.spec.ts \
  e2e/tests/settings/agent-runtime-update-helpers.ts \
  e2e/tests/settings/copilot-profile-plugins.spec.ts \
  e2e/tests/settings/mobile-agent-runtime-update-copilot.spec.ts \
  e2e/tests/settings/mobile-copilot-profile-plugins.spec.ts \
  lib/api/domains/agent-update-api.ts \
  lib/session-last-agent-error.test.ts \
  lib/session-last-agent-error.ts \
  lib/types/copilot-plugin-discovery.ts \
  lib/types/task-status-summary.ts)
```

### Coordinator gates

Catalog and all-spec lint passed after the accepted design became current:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

The catalog accepted 432 decisions and 1921 specifications. The final local
PR-documentation preflight compared the working tree with the verified PR merge
base `360b05ccd378cbe8c08bfcdc86a9c9a9d3465fd7`. It returned `covered`:
94 changed paths, 25 triggering paths, five work orders, 61 acceptance references,
ten referenced documents, and valid dependencies for all five work orders.
There were no reference errors. All 81 relative file targets in 14 changed
Markdown files exist. Tracked and plan-specific whitespace checks passed;
all 12 untracked text files passed whitespace, final-newline, and conflict checks.
All package documents remain below the 32768-byte limit.

At the implementation handoff, the completed local amendment was uncommitted
and PR #5078 pointed to `92ce0f8ffd4b8c8f85b669be3e01696a6504d678`.
Real native verification from task 04 covers Linux/amd64 only;
Darwin native execution was not run. The new browser tests use disposable CLI
fixtures, and the MCP regression covers the shared request paths described above.

### Reproduce coordinator reference and whitespace checks

Run from the repository root:

```bash
node <<'JS'
const {execFileSync} = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path').posix;
const gate = require('./.github/scripts/pr-docs.cjs');
const gitPaths = args => execFileSync('git', args, {encoding:'utf8'}).split('\0').filter(Boolean);
const tracked = gitPaths(['diff','--name-only','-z','360b05ccd378cbe8c08bfcdc86a9c9a9d3465fd7']);
const untracked = gitPaths(['ls-files','--others','--exclude-standard','-z']);
const changes = [...new Set([...tracked,...untracked])].sort().map(filename => ({filename,status:untracked.includes(filename)?'added':'modified'}));
const contents = {};
const load = p => contents[p] ??= fs.readFileSync(p,'utf8');
const frontmatter = p => gate.parseFrontmatter(load(p)).data;
const orders = gate.selectChangedWorkOrders(changes);
for (const order of orders) {
  const data = frontmatter(order);
  const plan = path.normalize(path.join(path.dirname(order),data.plan));
  const planData = frontmatter(plan);
  for (const ref of data.system_design) load(path.normalize(path.join(path.dirname(order),ref)));
  for (const ref of planData.system_design) load(path.normalize(path.join(path.dirname(plan),ref)));
}
load('docs/specs/agents/requirements/copilot-profile-plugins.md');
const result = gate.validateCoverage({changedFiles:changes,fileContents:contents});
const directory = 'docs/plans/copilot-runtime-plugin-compatibility';
const allOrders = fs.readdirSync(directory).filter(name => /^task-.*\.md$/.test(name)).map(name => ({path:path.join(directory,name),data:gate.parseFrontmatter(fs.readFileSync(path.join(directory,name),'utf8')).data}));
const ids = new Set(allOrders.map(order=>order.data.id));
const dependencyErrors = [];
if(ids.size!==allOrders.length) dependencyErrors.push('duplicate work-order id');
for(const order of allOrders) for(const dependency of order.data.depends_on) if(!ids.has(dependency)) dependencyErrors.push(`${order.path}: unknown dependency ${dependency}`);
console.log(JSON.stringify({status:result.status,changedPaths:changes.length,triggeringPaths:result.triggeringPaths.length,changedWorkOrders:orders.length,acceptanceReferences:orders.reduce((sum,p)=>sum+frontmatter(p).acceptance_criteria.length,0),referencedDocuments:Object.keys(contents).length,workOrdersWithValidDependencies:allOrders.length,errors:[...result.errors,...dependencyErrors]},null,2));
if(!result.ok || dependencyErrors.length) process.exitCode=1;
JS
python3 - <<'PY'
import json,re,subprocess
from pathlib import Path
from urllib.parse import unquote,urlsplit
root=Path.cwd().resolve()
base='360b05ccd378cbe8c08bfcdc86a9c9a9d3465fd7'
paths=set(subprocess.check_output(['git','diff','--name-only','-z',base],text=True).split('\0'))
paths.update(subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],text=True).split('\0'))
files=sorted(p for p in paths if p.startswith('docs/') and p.endswith('.md') and Path(p).is_file())
checked=0
errors=[]
for name in files:
    source=Path(name)
    for match in re.finditer(r'\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)',source.read_text()):
        target=match.group(1).strip('<>')
        url=urlsplit(target)
        if url.scheme or target.startswith('#') or not url.path:
            continue
        resolved=(root/url.path.lstrip('/') if target.startswith('/') else source.parent/unquote(url.path)).resolve()
        if not resolved.is_relative_to(root) or not resolved.exists():
            errors.append(f'{name}: {target}')
        checked+=1
print(json.dumps({'markdownFiles':len(files),'relativeLinks':checked,'errors':errors},indent=2))
raise SystemExit(bool(errors))
PY
git diff --check
git diff --check -- docs/plans/copilot-runtime-plugin-compatibility
git status --short -- docs/plans/copilot-runtime-plugin-compatibility
python3 - <<'PY'
import json,re,subprocess
from pathlib import Path
paths=subprocess.check_output(['git','ls-files','--others','--exclude-standard'],text=True).splitlines()
errors=[]
checked=[]
for name in paths:
    p=Path(name)
    if p.suffix not in {'.go','.ts','.tsx','.mts','.cts','.json','.md'}:
        continue
    text=p.read_text()
    checked.append(name)
    if not text.endswith('\n'):
        errors.append(f'{name}: missing final newline')
    for line_number,line in enumerate(text.splitlines(),1):
        if line.rstrip(' \t')!=line:
            errors.append(f'{name}:{line_number}: trailing whitespace')
        if re.match(r'^(<<<<<<<|=======|>>>>>>>)',line):
            errors.append(f'{name}:{line_number}: conflict marker')
sizes={str(p):p.stat().st_size for p in Path('docs/plans/copilot-runtime-plugin-compatibility').glob('*.md')}
if any(size>32768 for size in sizes.values()):
    errors.append('package document exceeds 32768 bytes')
print(json.dumps({'untrackedTextFiles':len(checked),'packageBytes':sizes,'errors':errors},indent=2))
raise SystemExit(bool(errors))
PY
```
