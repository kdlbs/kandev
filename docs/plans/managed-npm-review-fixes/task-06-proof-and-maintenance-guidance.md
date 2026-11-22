---
id: "06-proof-and-maintenance-guidance"
title: "Prove integrated recovery and document safe maintenance"
status: done
wave: 6
depends_on:
  - "05-cancellable-toolchain-context"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.15
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.16
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.18
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.20
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 06: Prove integrated recovery and document safe maintenance

## Summary

Prove the combined correction with local npm and existing user/executor fixtures.
Document the exact repair-store location and the stopped-consumer removal procedure.

## In scope

- Add real-npm proof for the incident package/version/flag with a loopback registry and a protocol-only fixture.
- Cover first recovery, later reuse, rejected duplicate Start, background-owner parity, and unchanged physical-cache evidence.
- Extend existing desktop/phone session and settings flows. Use named `managed npm review fixes` cases.
- Extend Docker/SSH retained-storage scenarios with valid flags, a cold owned process, and no second installer/archive download.
- Update `docs/public/agents-and-profiles.md` with location, retained disk use, and safe explicit removal guidance.
- Name the default `~/.kandev/managed-runtime-repairs/v1/` path and the override form. Avoid a blanket promise of zero npm metadata requests.
- Reconcile companion plan/result links and PR claims against actual final evidence. Refresh current PR and main heads before later integration.

## Out of scope

- Live-instance maintenance, automatic cleanup, a storage-size advisory change, new UI, product settings, or public npm/model requests.

## Acceptance

1. `TestManagedRuntimeRepairReuseNPMWithCLIArgs` first reproduces the missing-root failure for the incident descriptor. The repaired and reused process retain argv and initialize without paid model inference. Reuse downloads no archive and starts one provider.
2. Desktop, phone, Docker, and SSH cases preserve native conversation and permission arguments, unchanged shared sentinels, bounded attempts, and one accepted follow-up. Same-storage restart proves reuse rather than live adoption. A new storage lifetime remains a normal miss.
3. Public guidance names `<KANDEV_HOME_DIR>/managed-runtime-repairs/v1/` and the default execution-host home equivalent. It requires stopping all agents, utility/installer processes, agentctl owners, and automatic relaunch before removing only that exact store. It preserves `.npm`, provider state, and the rest of Kandev home. Documentation and package coverage gates pass.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5 ./internal/agent/managedruntime -run 'TestManagedRuntimeRepairReuseNPM|TestManagedRuntimeRepairReuseNPMWithCLIArgs' -count=1)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/managed-npm-cache-recovery.spec.ts tests/settings/managed-npm-cache-recovery.spec.ts --grep 'managed npm review fixes')
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-managed-npm-cache-recovery.spec.ts tests/settings/mobile-managed-npm-cache-recovery.spec.ts --grep 'managed npm review fixes')
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/managed-runtime-npm-recovery.spec.ts tests/ssh/managed-runtime-npm-recovery.spec.ts --grep 'managed npm review fixes')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs
```

Install dependencies with `(cd apps && pnpm install --frozen-lockfile)` only when the worktree lacks them.
The managed E2E runner builds isolated artifacts and owns their teardown.
Never direct these fixtures at the live Kandev instance.

Run this package-coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const path = require('node:path');
const cp = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/managed-npm-review-fixes';
const packagePaths = fs.readdirSync(dir).filter(n => n.endsWith('.md')).map(n => path.posix.join(dir, n));
const contracts = [
  'docs/specs/agents/requirements/managed-npm-runtime-recovery.md',
  'docs/specs/agents/requirements/managed-npm-cache-recovery.md',
  'docs/specs/agents/requirements/managed-npm-repair-reuse.md',
  'docs/specs/agents/system-design/managed-npm-runtime-recovery.md',
  'docs/specs/agents/system-design/managed-npm-cache-recovery.md',
  'docs/specs/agents/system-design/managed-npm-repair-reuse.md',
];
const tracked = cp.execFileSync('git', ['diff', '--name-only'], {encoding:'utf8'}).trim().split('\n').filter(Boolean);
const added = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding:'utf8'}).trim().split('\n').filter(Boolean);
const paths = [...new Set([...packagePaths, ...contracts, ...tracked, ...added])];
const fileContents = Object.fromEntries(paths.filter(p => p.endsWith('.md')).map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...new Set([...paths, 'apps/backend/internal/agent/managedruntime/prepared_runtime.go'])].map(filename => ({filename, status: added.includes(filename) ? 'added' : 'modified'}));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify({ok:result.ok, workOrders:result.workOrders, errors:result.errors}, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

The projected `prepared_runtime.go` path exercises the implementation coverage gate during planning.
It does not change that file.

After implementation, lint the actual changed Go files and record the invocation:

```bash
python3 <<'PY'
import subprocess
changed = set(subprocess.check_output(['git', 'diff', '--name-only', '--', '*.go'], text=True).splitlines())
changed.update(subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '--', '*.go'], text=True).splitlines())
if changed:
    subprocess.run(['bash', 'scripts/lint-go-changed', *sorted(changed)], check=True)
PY
```


## Files likely touched

- apps/backend/internal/agent/managedruntime/repair_reuse_npm_test.go and local archive fixture helpers
- apps/web/e2e/helpers/managed-runtime-recovery.ts
- apps/web/e2e/helpers/managed-npm-cache-recovery-scenarios.ts
- apps/web/e2e/helpers/managed-npm-probe-recovery-scenarios.ts
- apps/web/e2e/fixtures/managed-runtime-npx.sh and related owned fixture scripts
- apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts
- apps/web/e2e/tests/ssh/managed-runtime-npm-recovery.spec.ts
- docs/public/agents-and-profiles.md
- This package and affected companion plan links/results

## Dependencies

Task 05.

## Risks

- Backend shutdown alone can leave surviving or remote agents using the store.
- Default-authentication fixtures must not inject a custom owner or fake install context that hides the background-launch defect.
- Missing executor infrastructure is missing evidence. An empty grep selection is not a pass.
- Windows currently uses private recovery because reusable-store ACL support is disabled. Preserve that boundary and avoid an unsupported reuse claim.

## Parallelism

`sequential`

## Inputs

- Review findings 4 and 5, incident evidence, and Tasks 01 through 05.
- Existing desktop/mobile session/settings and Docker/SSH same-storage proof.
- `docs/public/README.md` and the current runtime/storage advisory contract.

## Results

Done on 2026-11-09.

- Added the real-npm incident-shaped `@agentclientprotocol/claude-agent-acp@0.100.0` regression with `--dangerously-skip-permissions`; it now captures the provider argv and verifies the flag exactly once on the repaired and cold-reused launches, one recovery, unchanged resume token, and no further installer or archive download.
- Added desktop and phone `managed npm review fixes` session and settings cases with profile flags; all runs passed, including provider argv capture, required resume-token continuity, and one accepted follow-up.
- Extended Docker and SSH retained-storage cases with valid profile flags and execution-host evidence: captured provider argv, installer calls, provider PIDs, required resume tokens, shared/sibling sentinels, bounded starts, a distinct replacement PID, one accepted follow-up, and unchanged installer/archive counts. The containers project passed all three Docker and all three SSH cases, including same-storage cold reuse and the new-container miss control. The fixture kills only the observed task-owned provider tree, not every mock agent.
- Documented `<KANDEV_HOME_DIR>/managed-runtime-repairs/v1/`, the default `~/.kandev/managed-runtime-repairs/v1/`, stopped-consumer requirements, and exact-store removal in `docs/public/agents-and-profiles.md`.
- Public-doc validators, spec catalog validation, specification lint, and whitespace checks passed.
- Changed-Go lint and a clean sequential lint rerun passed after remediation.
- New POSIX shell fixtures skip on Windows; Windows cross-compilation of the affected Go test packages passed.
