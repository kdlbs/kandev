---
id: "04-review-misc-workflows"
title: "Upgrade Node-20 actions in review and misc workflows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
---

# Task 04: Upgrade Node-20 actions in review and misc workflows

- **Acceptance:** every `uses:` pin in `claude.yml`, `claude-code-review.yml`, `opencode-code-review.yml`, `notify-docs.yml`, `preview-env.yml`, `pr-title.yml`, and `plugin-registry-index.yml` runs on Node 24; the claude-code-review and preview-env workflow contract tests still pass.
- **Verification:**
  - `python3 .github/scripts/lint-action-pinning.py`
  - `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  - `python3 .github/scripts/preview-env-workflow-contract_test.py`
  - runtime audit per `audit.md` for the seven files
  - `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/claude.yml .github/workflows/claude-code-review.yml .github/workflows/opencode-code-review.yml .github/workflows/notify-docs.yml .github/workflows/preview-env.yml .github/workflows/pr-title.yml .github/workflows/plugin-registry-index.yml`
- **Files likely touched:**
  - `.github/workflows/claude.yml` (checkout:29 → v6)
  - `.github/workflows/claude-code-review.yml` (checkout:43,128 → v6; github-script:92 → v9.0.0)
  - `.github/workflows/opencode-code-review.yml` (checkout:37,293 → v6; upload-artifact:236,476 → v7.0.1; github-script:255 → v9.0.0)
  - `.github/workflows/notify-docs.yml` (github-script:146 → v9.0.0; wrangler-action:120 → v4.0.0)
  - `.github/workflows/preview-env.yml` (github-script:156 → v9.0.0)
  - `.github/workflows/pr-title.yml` (semantic-pull-request:20 → v6.1.1)
  - `.github/workflows/plugin-registry-index.yml` (configure-pages:81 → v6.0.0; upload-pages-artifact:85 → v5.0.0; deploy-pages:103 → v5.0.0)
- **Dependencies:** None.
- **Parallelism:** sequential by default; parallel-safe if run alongside tasks 01–03 (all files disjoint).

## Changes

| File:line | From | To |
|---|---|---|
| claude.yml:29 | `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4` | `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6` |
| claude-code-review.yml:43,128 | `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4` | `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6` |
| claude-code-review.yml:92 | `actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | `actions/github-script@3a2844b7e9c422d3c10d287c895573f7108da1b3 # v9.0.0` |
| opencode-code-review.yml:37,293 | `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4` | `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6` |
| opencode-code-review.yml:236,476 | `actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4` | `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1` |
| opencode-code-review.yml:255 | `actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | `actions/github-script@3a2844b7e9c422d3c10d287c895573f7108da1b3 # v9.0.0` |
| notify-docs.yml:146 | `actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | `actions/github-script@3a2844b7e9c422d3c10d287c895573f7108da1b3 # v9.0.0` |
| notify-docs.yml:120 | `cloudflare/wrangler-action@9acf94ace14e7dc412b076f2c5c20b8ce93c79cd # v3` | `cloudflare/wrangler-action@ebbaa1584979971c8614a24965b4405ff95890e0 # v4.0.0` |
| preview-env.yml:156 | `actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | `actions/github-script@3a2844b7e9c422d3c10d287c895573f7108da1b3 # v9.0.0` |
| pr-title.yml:20 | `amannn/action-semantic-pull-request@e32d7e603df1aa1ba07e981f2a23455dee596825 # v5` | `amannn/action-semantic-pull-request@48f256284bd46cdaab1048c3721360e808335d50 # v6.1.1` |
| plugin-registry-index.yml:81 | `actions/configure-pages@983d7736d9b0ae728b81ab479565c72886d7745b # v5` | `actions/configure-pages@45bfe0192ca1faeb007ade9deae92b16b8254a0d # v6.0.0` |
| plugin-registry-index.yml:85 | `actions/upload-pages-artifact@56afc609e74202658d3ffba0e8f6dda462b719fa # v3` | `actions/upload-pages-artifact@fc324d3547104276b827a68afc52ff2a11cc49c9 # v5.0.0` |
| plugin-registry-index.yml:103 | `actions/deploy-pages@d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e # v4` | `actions/deploy-pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128 # v5.0.0` |

Line numbers are 2026-08-07 snapshots; match by `uses:` string if they moved.

Notes:
- github-script v9.0.0: the four scripts only use injected `github`/`context`
  (labels, removeLabel, safe-to-test) — no `require('@actions/github')`, so
  the v9 breaking change does not apply.
- wrangler-action v4.0.0: keep `wranglerVersion: 3.90.0` in the `with:` block
  (v4 defaults to Wrangler v4; the explicit pin preserves current behavior).
- plugin-registry-index.yml Pages trio: bump all three together — v5
  upload-pages-artifact's inner `actions/upload-artifact` is v7.0.0 (node24)
  instead of v4 (node20). The `path: _site` input is unchanged.

## Inputs

plan.md target table + compatibility notes; `audit.md`.

## Output contract

Summary, files changed, exact commands and outcomes, audit output for the seven files, task/plan status updates.

## Results

- Applied 6 checkout v4→v6, 3 github-script v7→v9.0.0, 2 upload-artifact v4→v7.0.1, 1 wrangler v3→v4.0.0, 1 semantic-pull-request v5→v6.1.1, 3 Pages trio upgrades across claude.yml / claude-code-review.yml / opencode-code-review.yml / notify-docs.yml / preview-env.yml / pr-title.yml / plugin-registry-index.yml.
- github-script scripts audited: only injected `github`/`context`, no `require('@actions/github')` → v9.0.0 safe.
- `wranglerVersion: 3.90.0` kept in notify-docs.yml (v4 default avoided).
- `python3 .github/scripts/lint-action-pinning.py`, `claude-code-review-workflow-contract_test.py` (9 OK), `preview-env-workflow-contract_test.py` (1 OK) all pass.
- Runtime audit: all pins `node24`/`composite`, zero `node20`; YAML parse OK; `git diff --check` clean. None.
