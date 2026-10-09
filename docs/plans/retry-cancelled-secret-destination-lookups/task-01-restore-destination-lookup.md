---
id: "01-restore-destination-lookup"
title: "Restore canceled destination lookup recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001
acceptance_criteria:
  - AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.9
  - AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.10
  - AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.11
system_design:
  - ../../specs/workspaces/system-design/secret-scope-transfer.md
---

# Task 01: Restore canceled destination lookup recovery

## Summary

Correct the hook-local reusable key so canceling a pending destination lookup cannot suppress the replacement lookup. Prove recovery through the real hook and rendered Copy/Move consumer without replacing production APIs, store, predicates, hooks, or markup.

## In scope

- Apply the owning design's [destination-name lifecycle](../../specs/workspaces/system-design/secret-scope-transfer.md#destination-name-lookup-lifecycle) within the existing hook and effect-local cancellation fence.
- Add the two new suites listed below; retain existing focused tests. Fixtures use real StateProvider/createAppStore, APIs and locale setup; only fetch is deferred.
- Keep this plan, work-order results, and owner pair synchronized with actual implementation and exact gate outcomes.

## Out of scope

No backend, API/types/schema, global cache, abort framework, new loading restriction, copy, layout, touch, scrolling, navigation, or breakpoint changes. No permanent tests or production edits during design. Consumer production glue requires new causal evidence and ROOT scope checkpoint. No installs/product tests during design; no browser/E2E/build/backend/DB/broad suite, baseline replay, optional polish, or main-only rebase during implementation.

## Acceptance

1. The pending roundtrip and standalone StrictMode tests fail causally against unchanged production, then pass with the local hook correction; the faithful rendered pending roundtrip obtains a replacement response and restores the existing duplicate pre-check.
2. Real production fixtures preserve completed-cache reuse, current success/failure, refreshKey, changed-workspace isolation, Global store names, dialog new-session refresh, and current 409/generic transfer errors. Abandoned success, rejection, and finalization cannot publish or mark current state loaded/cached.
3. All exact scoped gates below actually join within caps and pass. Record actual outputs/counts and owned-file hashes; normal hooks and publication follow only the later release. Marking this work order done does not imply CI/merge/task completion.

## Test construction and TDD order

After ROOT's later SAME-primary implementation interrupt and exclusive LOCAL-HEAVY admission, read this work order and current source, mark `in_progress`, and check dependency presence. Install once only if absent using the pinned command below; preserve managed dependencies and caches.

Add `hooks/domains/settings/use-secret-destination-names.lifetime.test.tsx` with describe `cancelled destination lookup recovery`. Import the real hook and `StateProvider`; wrap with the actual provider, which creates `createAppStore`. Use Testing Library root `reactStrictMode: true` with the real provider for `restarts the standalone StrictMode setup`. Deferred fetch tickets record URL/init, return actual JSON Responses with full SecretListItem metadata, and support rejection/HTTP failures. Test `replaces a pending lookup after a scope roundtrip`: admit A, switch Global, return A before the first response settles, assert a replacement GET with workspace scope/query and no-store, resolve current duplicate, then abandoned response (and inverse order in a control), observe current names/loaded/conflict. Repeat stale rejection/finalization controls. Completed current lookup roundtrip uses one read; refreshKey causes a fresh read; B rejects late A publication; Global names use actual store mutation. A current failure remains loaded with empty names and no invented conflict, including completed fallback reuse. No missing-ID or new retry policy is added.

Add `components/settings/copy-move-secret-dialog.destination-names.test.tsx` with the same describe prefix. Hydrate full source workspace A and destination B through StateProvider initialState or actual useAppStoreApi actions; preload the real workspace collection so unrelated destination discovery is not fetched. Render the actual CopyMoveSecretDialog with a Workspace-scoped source and real body/Radix controls. No module substitutions. For `restores the rendered duplicate pre-check after a pending roundtrip`, use the actual labeled combobox/options to choose B, General (current English locale displays Global), then B while the first B GET is pending. Resolve replacement GET with the entered trimmed duplicate. Assert actual field `aria-invalid=true`, `aria-describedby` points to the existing localized conflict message, and the primary button is disabled; changing to an available name clears invalidity and enables the primary action. The abandoned response cannot undo the current duplicate. Also control completed B reuse, reopening with refreshKey, current lookup failure allowing a valid submit, current transport 409 mapping to name error, and non-409 mapping to generic alert; actual transfer requests preserve source workspace query and target payload. Pending lookup alone does not disable submit.

Cleanup unmounts the real tree, settles every fixture-owned pending fetch, awaits effect completion with act, then restores fetch. Use causal event/response completion; no sleep, fixed microtask-count assumptions, extra timeout, assertion weakening, exported private helpers, fake dialog, or API/predicate/hook/store mocks. Avoid replacing Radix primitives; existing suite event patterns and setup provide the supported seam. Current-success tests should exercise both zero-conflict and duplicate outcomes. Fixtures/tests stay within existing lint limits.

Run `red` only against the new principal hook/dialog/StrictMode regression names; expect assertion failures for missing replacement fetch/conflict recovery, not setup, timeout, transport, or unrelated failures. Accepted ROOT proof is read-only and is never replayed. Then make the minimal production correction and run `targeted` plus remaining scoped gates once. Routine causal own fixture/lint fixes rerun only affected gates. Resource/timeout/transport/unknown/out-of-scope failure checkpoints ROOT, with no automatic recovery or larger caps.

## Verification

Run from the repository root in a retained `exec_command` with `shell="/bin/bash"`, `login=false`. Record its actual returned session handle, wrapper PID/start/log and deadline before doing anything else. Every gate below uses a new owned process group and an upfront receipt; poll the same returned handle to actual terminal completion and verify owned groups gone before the next tool action. No concurrent local commands or dropped handles.

The block is the exact future command recipe, not run in design. `KANDEV_SECRET_GATE=red` runs only RED (after the conditional install); default `green` runs the targeted green suite and scoped gates. `KANDEV_SECRET_GATE=<gate-name>` runs only the affected gate for causal remediation. All paths are rooted independently. Existing Node 24 path is command-local; no global runtime, lockfile, hook, or cache edits. Typecheck uses its normal pretypecheck generators once; unexpected dirty output checkpoints ROOT.

```bash
KANDEV_SECRET_GATE=green python3 - <<'PYRUN'
import datetime, json, os, pathlib, signal, subprocess, sys, time
root = pathlib.Path.cwd()
node_bin = '/home/jcfs/.nvm/versions/node/v24.18.0/bin'
env = dict(os.environ, PATH=node_bin + ':' + os.environ['PATH'], NODE_OPTIONS='--max-old-space-size=4096')
pnpm = [node_bin + '/corepack', 'pnpm@9.15.9']
mode = os.environ.get('KANDEV_SECRET_GATE', 'green')
new_tests = ['hooks/domains/settings/use-secret-destination-names.lifetime.test.tsx', 'components/settings/copy-move-secret-dialog.destination-names.test.tsx']
existing_tests = ['hooks/domains/settings/use-secret-destination-names.test.ts', 'hooks/domains/settings/use-workspace-destinations.test.ts', 'components/settings/copy-move-secret-dialog.test.tsx', 'lib/api/domains/secrets-api.test.ts']
vitest = pnpm + ['exec', 'vitest', 'run', '--project', 'browser-locales', '--maxWorkers=1', '--no-file-parallelism']
owned = ['docs/specs/workspaces/requirements/secret-scope-transfer.md', 'docs/specs/workspaces/system-design/secret-scope-transfer.md', 'docs/plans/retry-cancelled-secret-destination-lookups/plan.md', 'docs/plans/retry-cancelled-secret-destination-lookups/task-01-restore-destination-lookup.md']
coverage = "const fs=require('node:fs'),cp=require('node:child_process'),{validateCoverage}=require('./.github/scripts/pr-docs.cjs');const paths=[...cp.execFileSync('git',['diff','--name-only','HEAD'],{encoding:'utf8'}).trim().split('\\n'),...cp.execFileSync('git',['ls-files','--others','--exclude-standard'],{encoding:'utf8'}).trim().split('\\n')].filter(Boolean);const docs=" + json.dumps(owned) + ";const fileContents=Object.fromEntries(docs.map(p=>[p,fs.readFileSync(p,'utf8')]));const r=validateCoverage({changedFiles:paths.map(filename=>({filename,status:'modified'})),fileContents});console.log(JSON.stringify(r,null,2));if(!r.ok)process.exit(1);"
gates = [
 ('red', 'apps/web', 120, vitest + new_tests + ['-t', 'replaces a pending lookup after a scope roundtrip|restarts the standalone StrictMode setup|restores the rendered duplicate pre-check after a pending roundtrip']),
 ('targeted', 'apps/web', 120, vitest + new_tests + existing_tests),
 ('eslint', 'apps/web', 120, pnpm + ['exec', 'eslint', '--max-warnings', '0', 'hooks/domains/settings/use-secret-destination-names.ts'] + new_tests),
 ('typecheck', 'apps/web', 180, pnpm + ['run', 'typecheck']),
 ('i18n-check', 'apps/web', 120, pnpm + ['run', 'i18n:check']),
 ('i18n-ratchet', 'apps/web', 120, pnpm + ['run', 'i18n:ratchet']),
 ('public-test', '.', 60, [node_bin + '/node', '--test', 'scripts/validate-public-docs.test.mjs']),
 ('public', '.', 60, [node_bin + '/node', 'scripts/validate-public-docs.mjs']),
 ('catalog', '.', 60, ['python3', 'scripts/list-docs.py', 'validate']),
 ('spec-test', '.', 60, ['python3', 'scripts/lint-spec-files.test.py']),
 ('spec', '.', 60, ['python3', 'scripts/lint-spec-files.py', '--all']),
 ('coverage', '.', 60, [node_bin + '/node', '-e', coverage]),
 ('whitespace', '.', 60, ['git', 'diff', '--check']),
]
if not (root / 'apps/node_modules').is_dir():
 gates.insert(0, ('install', 'apps', 300, pnpm + ['install', '--frozen-lockfile']))
selected = [g for g in gates if g[0] == 'install' or (mode == 'green' and g[0] != 'red') or g[0] == mode]
if not selected: raise SystemExit('Unknown gate; checkpoint ROOT')
for name, cwd, cap, argv in selected:
 started = datetime.datetime.now(datetime.timezone.utc)
 stamp = str(time.time_ns())
 log = pathlib.Path('/tmp') / ('kandev-child45-' + name + '-' + stamp + '.log')
 receipt = log.with_suffix('.json')
 with log.open('wb') as out:
  proc = subprocess.Popen(argv, cwd=root / cwd, env=env, stdout=out, stderr=subprocess.STDOUT, start_new_session=True)
  data = dict(gate=name, command=argv, cwd=str(root / cwd), wrapper_pid=os.getpid(), pid=proc.pid, process_group=proc.pid, start=started.isoformat(), cutoff=(started + datetime.timedelta(seconds=cap)).isoformat(), cap_seconds=cap, log=str(log), joined=False)
  receipt.write_text(json.dumps(data, indent=2))
  print(json.dumps(data), flush=True)
  try: code = proc.wait(timeout=cap)
  except subprocess.TimeoutExpired:
   os.killpg(proc.pid, signal.SIGTERM)
   try: proc.wait(timeout=5)
   except subprocess.TimeoutExpired:
    os.killpg(proc.pid, signal.SIGKILL); proc.wait()
   code = 124
  try: os.killpg(proc.pid, 0); gone = False
  except ProcessLookupError: gone = True
  data.update(exit_code=code, joined=True, process_group_gone=gone, end=datetime.datetime.now(datetime.timezone.utc).isoformat())
  receipt.write_text(json.dumps(data, indent=2))
  print(json.dumps(data), flush=True)
 if code or not gone: raise SystemExit(code or 1)
PYRUN
```

RED exits nonzero by design; inspect the saved log to classify causal assertion failures before changing production. The owned-group timeout termination above is only the predefined cutoff, not permission to recover or rerun. Record the real tool handle in task-plan evidence alongside each receipt. No foreign resources are inspected or killed.

Design uses only public/spec/catalog/coverage/actual-path/whitespace checks against the four actual files; Node validators are dependency-free. Untracked Markdown is checked separately because git diff does not include it. For final design handoff and after later scoped gates, inspect actual `git status --short`, `git diff --cached --name-only`, all owned SHA256/Git blobs, and current HEAD. No invented passing count, prospective-path validation verdict, or lost-handle completion claim.

## Files likely touched

- `apps/web/hooks/domains/settings/use-secret-destination-names.ts`
- `apps/web/hooks/domains/settings/use-secret-destination-names.lifetime.test.tsx` (new after release)
- `apps/web/components/settings/copy-move-secret-dialog.destination-names.test.tsx` (new after release)
- `docs/specs/workspaces/requirements/secret-scope-transfer.md`
- `docs/specs/workspaces/system-design/secret-scope-transfer.md`
- `docs/plans/retry-cancelled-secret-destination-lookups/plan.md`
- `docs/plans/retry-cancelled-secret-destination-lookups/task-01-restore-destination-lookup.md`

## Dependencies

None. ROOT actual-file review and later SAME-primary implementation interrupt with explicit exclusive LOCAL-HEAVY release are admission barriers, not inferred from the plan. No resource lease claimed during design.

## Risks and delivery

See the manifest's [delivery barriers](plan.md#design-checkpoint-and-later-delivery-barriers), including normal hooks/new Conventional Commit/no bypass, exact-head publication/freeze, one 45-minute collector, App 347564 substantive FULL all-files review, six required plus actual Backend/Frontend/E2E parent successes, fresh resolver/governance and hidden/human gates, and separate ROOT serial merge grant. Preserve original proof, managed checkout/dependencies/shared caches/foreign resources. Callback/question queue full: checkpoint critical barriers in existing plan/conversation, END WAITING, never retry callbacks/questions or invent approvals/leases. Machine crash absence is not guaranteed.

## Parallelism

`sequential`. No delegation, persistent task/session/tab creation, or model switches.

## Inputs

- [Requirements](../../specs/workspaces/requirements/secret-scope-transfer.md), AC 001.9-001.11.
- [System design](../../specs/workspaces/system-design/secret-scope-transfer.md#destination-name-lookup-lifecycle).
- [Manifest](plan.md), admission evidence and original completed companion package.
- Actual hook, provider/store/API/types, dialog/body, workspace destination hook, existing focused suites, real Vitest locale setup and Radix event patterns.

## Results

Implemented after ROOT actual-file review and later SAME-primary release. Only production change: the existing hook invalidates reusable admission state and promotes the workspace key at accepted current finalization. Both declared suites use actual production dependencies and only deferred fetch.

- One frozen pinned apps install succeeded in 1.9s; no dependency/lock/hook/runtime edits.
- Initial permanent RED (handle 48348, `/tmp/kandev-child45-red-1791267995324612182.json`) failed the real hook roundtrip and rendered dialog roundtrip with one request instead of two. Initial subtree StrictMode fixture was insufficient; root StrictMode was then enabled using Testing Library's supported option. Corrected standalone StrictMode RED against restored original hook (handle 97068, `/tmp/kandev-child45-red-1791268080843575500.json`) also failed the missing replacement request assertion. Historical ROOT proof was not replayed or edited.
- First six-suite GREEN attempt (handle 22446) passed 50/51; the sole failure was the subtree StrictMode fixture. All four existing relevant suites (34 tests) and the rendered suite passed. Only affected new suites were rerun after root StrictMode, lint structure/constants, and fixture typing repairs. Final two-suite run (handle 46153, `/tmp/kandev-child45-targeted-1791268360459048944.json`) passed 17/17. No passing existing-suite replay.
- Changed ESLint max-warnings 0 passed after splitting overlong describe callbacks and extracting repeated attribute/label constants. Typecheck initially reported only own fixture errors (unsupported role-query `exact` and inferred required workspaceId). Those were corrected; affected normal typecheck rerun passed under the original 180s cap. Role names retain string-exact matching. No assertion weakening, resource recovery, larger bounds, or production expansion.
- Normal typecheck (including generators), i18n:check, i18n:ratchet, public validator tests/pages, catalog validation, spec-validator tests, full spec lint, actual seven-path coverage (`covered`), and whitespace passed. Receipts and each raw log are indexed in `/tmp/kandev-child45-local-gate-ledger.json`; handle 48071 actually joined exit 0 with all owned groups gone.

Normal commit/publication and hosted review/check/merge gates remain separately pending. All previous refusals/model_capacity history remain evidence; no machine-crash cause is inferred. GLOBAL LOCAL-HEAVY remains held through publication proof; MERGE NONE.
