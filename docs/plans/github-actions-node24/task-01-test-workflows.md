---
id: "01-test-workflows"
title: "Upgrade Node-20 actions in test workflows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
---

# Task 01: Upgrade Node-20 actions in test workflows

- **Acceptance:** every `uses:` pin in `backend-tests.yml`, `frontend-tests.yml`, `cargo-audit.yml`, and `e2e-tests.yml` that previously ran on Node 20 now pins a commit whose `action.yml` declares `runs.using: node24`; the pinning lint and contract tests still pass.
- **Verification:**
  - `python3 .github/scripts/lint-action-pinning.py`
  - `python3 .github/scripts/lint-action-pinning_test.py`
  - `python3 .github/scripts/release-workflow-contract_test.py` (unchanged assertions still pass)
  - runtime audit for the four files per `audit.md` (all `runs.using` = node24/composite)
  - `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/cargo-audit.yml .github/workflows/e2e-tests.yml`
- **Files likely touched:**
  - `.github/workflows/backend-tests.yml` (cache:73,176 → v6.1.0; go-test-action:240 → v1.1.0; upload-artifact:247 → v7.0.1)
  - `.github/workflows/frontend-tests.yml` (cache:73 → v6.1.0)
  - `.github/workflows/cargo-audit.yml` (cache:53 → v6.1.0)
  - `.github/workflows/e2e-tests.yml` (cache:67,165,273,480 → v6.1.0; upload-artifact:96,103,114,211,219,384,392,509 → v7.0.1; download-artifact:176,185,191,345,354,360,491 → v8.0.1; login-action:288 → v4.6.0)
- **Dependencies:** None.
- **Parallelism:** sequential by default; parallel-safe if run alongside tasks 02–04 (all files disjoint).

## Changes

Replace each listed `uses:` line with the target SHA, keeping the `# vX` version comment accurate:

| File:line | From | To |
|---|---|---|
| backend-tests.yml:73,176 | `actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # v4` | `actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0` |
| backend-tests.yml:240 | `robherley/go-test-action@3856e4e57831b3b6d6d8d89f91747e5200c533f7 # v0` | `robherley/go-test-action@2f859e0c8769d755d3174eecb9af8f64660827f3 # v1.1.0` |
| backend-tests.yml:247 | `actions/upload-artifact@330a01c490aca151604b8cf639adc76d48f6c5d4 # v5` | `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1` |
| frontend-tests.yml:73 | `actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # v4` | `actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0` |
| cargo-audit.yml:53 | `actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # v4` | `actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0` |
| e2e-tests.yml:67,165,273,480 | `actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # v4` | `actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0` |
| e2e-tests.yml:96,103,114,211,219,384,392,509 | `actions/upload-artifact@330a01c490aca151604b8cf639adc76d48f6c5d4 # v5` | `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1` |
| e2e-tests.yml:176,185,191,345,354,360,491 | `actions/download-artifact@634f93cb2916e3fdff6788551b99b062d0335ce0 # v5` | `actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1` |
| e2e-tests.yml:288 | `docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3` | `docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0` |

Line numbers are 2026-08-07 snapshots; if a line moved, match by the `uses:`
string instead of the number.

Do not touch `e2e-tests.yml` checkout (already v6 `df4cb1c…`).

## Inputs

plan.md target table + compatibility notes; `audit.md` for the runtime check.

## Output contract

Summary, files changed, exact commands run and outcomes, final audit output for the four files, task/plan status updates.

## Results

- Applied 8 cache, 1 go-test-action, 9 upload-artifact, 7 download-artifact, 1 login-action replacements across backend-tests.yml / frontend-tests.yml / cargo-audit.yml / e2e-tests.yml (match-by-`uses:` verified).
- `python3 .github/scripts/lint-action-pinning.py` → "✓ All 18 workflow file(s) use SHA-pinned action refs."
- `python3 .github/scripts/lint-action-pinning_test.py` → 9 tests OK.
- `python3 .github/scripts/release-workflow-contract_test.py` → 24 tests OK (unchanged pins still asserted).
- Runtime audit (audit_final.py): every pin in the four files `node24`/`composite`, zero `node20`.
- YAML parse of all 18 workflows OK; `git diff --check` clean.
- No security/trust or external side-effect boundaries beyond publishing new action pins to CI.
