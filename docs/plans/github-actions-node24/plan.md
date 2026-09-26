---
created: 2026-08-07
status: draft
---

# Implementation Plan: Upgrade GitHub Actions to the Node 24 runtime

## Overview

GitHub deprecated the Node 20 action runtime; every pinned action whose
`action.yml` declares `runs.using: node20` now runs forced on Node 24 and
emits a deprecation warning on every workflow run. This plan upgrades every
Node-20 action pin in `.github/workflows/` to a SHA-pinned release that
declares `runs.using: node24`, verifying each target's runtime and
backwards-compatibility before landing. Order: upgrade the plain-JavaScript
actions (checkout, artifacts, cache, github-script) first, then the docker/*
family, then the remaining third-party actions, and finish with a full audit
that no `node20` pin remains.

There is no product spec for this change: it is CI-infrastructure hygiene with
no user-visible behavior. Validation is mechanical — see
[audit.md](audit.md) for the runtime-check procedure used to build the table
below.

## Target version table

Verified on 2026-08-07 via the pinned SHA's `action.yml` `runs.using` field
(`audit.md` documents the exact method). "Target" is the release whose pinned
commit declares `node24`.

| Action | Current pin | Uses | Target | Target SHA | Runtime |
|---|---|---|---|---|---|
| actions/checkout | `34e114876b0b11c390a56381ad16ebd13914f8d5 # v4` | 7 | v6 (already used in repo) | `df4cb1c069e1874edd31b4311f1884172cec0e10` | node24 |
| actions/upload-artifact | `ea165f8d65b6e75b540449e92b4886f43607fa02 # v4` | 2 | v7.0.1 | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` | node24 |
| actions/upload-artifact | `330a01c490aca151604b8cf639adc76d48f6c5d4 # v5` | 13 | v7.0.1 | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` | node24 |
| actions/download-artifact | `d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4` | 2 | v8.0.1 | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` | node24 |
| actions/download-artifact | `634f93cb2916e3fdff6788551b99b062d0335ce0 # v5` | 12 | v8.0.1 | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` | node24 |
| actions/cache | `0057852bfaa89a56745cba8c7296529d2fc39830 # v4` | 8 | v6.1.0 | `55cc8345863c7cc4c66a329aec7e433d2d1c52a9` | node24 |
| actions/github-script | `f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | 4 | v9.0.0 | `3a2844b7e9c422d3c10d287c895573f7108da1b3` | node24 |
| docker/login-action | `c94ce9fb468520275223c153574b00df6fe4bcc9 # v3` | 9 | v4.6.0 | `dbcb813823bdd20940b903addbd779551569679f` | node24 |
| docker/setup-buildx-action | `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3` | 8 | v4.2.0 | `bb05f3f5519dd87d3ba754cc423b652a5edd6d2c` | node24 |
| docker/build-push-action | `10e90e3645eae34f1e60eeb005ba3a3d33f178e8 # v6` | 7 | v7.3.0 | `53b7df96c91f9c12dcc8a07bcb9ccacbed38856a` | node24 |
| docker/metadata-action | `c299e40c65443455700f0fdfc63efafe5b349051 # v5` | 5 | v6.2.0 | `dc802804100637a589fabce1cb79ff13a1411302` | node24 |
| softprops/action-gh-release | `3bb12739c298aeb8a4eeaf626c5b8d85266b0e65 # v2` | 1 | v3.0.2 | `3d0d9888cb7fd7b750713d6e236d1fcb99157228` | node24 |
| cloudflare/wrangler-action | `9acf94ace14e7dc412b076f2c5c20b8ce93c79cd # v3` | 1 | v4.0.0 | `ebbaa1584979971c8614a24965b4405ff95890e0` | node24 |
| amannn/action-semantic-pull-request | `e32d7e603df1aa1ba07e981f2a23455dee596825 # v5` | 1 | v6.1.1 | `48f256284bd46cdaab1048c3721360e808335d50` | node24 |
| actions/deploy-pages | `d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e # v4` | 1 | v5.0.0 | `cd2ce8fcbc39b97be8ca5fce6e763baed58fa128` | node24 |
| actions/configure-pages | `983d7736d9b0ae728b81ab479565c72886d7745b # v5` | 1 | v6.0.0 | `45bfe0192ca1faeb007ade9deae92b16b8254a0d` | node24 |
| actions/upload-pages-artifact | `56afc609e74202658d3ffba0e8f6dda462b719fa # v3` | 1 | v5.0.0 | `fc324d3547104276b827a68afc52ff2a11cc49c9` | composite (inner upload-artifact v7.0.0 = node24) |
| robherley/go-test-action | `3856e4e57831b3b6d6d8d89f91747e5200c533f7 # v0` | 1 | v1.1.0 | `2f859e0c8769d755d3174eecb9af8f64660827f3` | node24 |

Already node24 / no change: `actions/checkout` v6 (`df4cb1c…`), `actions/setup-node` v6 (`48b55a0…`), `actions/setup-go` v6 (`924ae3a…`), `pnpm/action-setup` v5 (`a8198c4…`), `crazy-max/ghaction-import-gpg` v7.0.0 (`2dc316d…`), `actions/create-github-app-token` v3.2.0 (`bcd2ba4…`), `Swatinem/rust-cache` v2 (`42dc69e…`). Composite actions with no direct runtime (`anthropics/claude-code-action`, `orhun/git-cliff-action`, `dtolnay/rust-toolchain`) are left as-is.

## Compatibility notes (per action)

- `actions/checkout` v4→v6: v6 already used 34× in this repo — proven.
- `actions/upload-artifact` v4/v5→v7.0.1 and `actions/download-artifact` v4/v5→v8.0.1: the artifact backend is shared across the v4+ majors on GitHub-hosted runners; upload/download must stay on compatible majors (v7 upload + v8 download both use the v4 artifact API). All uses move together in the same PR, so cross-job download/upload pairing is preserved.
- `actions/github-script` v7→v9.0.0: v9 breaking change is `require('@actions/github')` no longer working; all four uses in this repo only use injected `github`/`context` — audited, no `require('@actions/github')` in any script block.
- `docker/login-action` v3→v4.6.0, `docker/setup-buildx-action` v3→v4.2.0: Node 24 runtime + ESM; v4 removed deprecated inputs/outputs — repo uses none (`with:` blocks checked).
- `docker/build-push-action` v6→v7.3.0: v7 removed deprecated `DOCKER_BUILD_NO_SUMMARY` / `DOCKER_BUILD_EXPORT_RETENTION_DAYS` envs — not used in repo.
- `docker/metadata-action` v5→v6.2.0: list inputs now preserve `#` inside values — release.yml uses no `#` in tags/images lists.
- `softprops/action-gh-release` v2→v3.0.2: Node 24 only; `files:`/`body_path:` inputs unchanged in v3.
- `cloudflare/wrangler-action` v3→v4.0.0: default wrangler version changes to v4 (`latest`), but repo pins `wranglerVersion: 3.90.0` explicitly — behavior preserved.
- `amannn/action-semantic-pull-request` v5→v6.1.1: Node 24 + ESM; inputs unchanged.
- `actions/deploy-pages` v4→v5.0.0, `actions/configure-pages` v5→v6.0.0, `actions/upload-pages-artifact` v3→v5.0.0: Pages trio moved together; v5 upload-pages-artifact's inner `actions/upload-artifact` is v7.0.0 (node24) instead of v4 (node20).
- `robherley/go-test-action` v0→v1.1.0: Node 24; `fromJSONFile` input retained.

## Tasks

Task files are grouped by workflow file so each task touches a disjoint set of
files (safe to run in any order; wave labels only express suggested order):

```
Wave 1 (parallel candidates — user authorization required):
- [x] [task-01-test-workflows](task-01-test-workflows.md)   — backend-tests.yml, frontend-tests.yml, cargo-audit.yml, e2e-tests.yml
- [x] [task-02-container-workflows](task-02-container-workflows.md) — universal-rebuild.yml, ci-base-image.yml
- [x] [task-03-release-workflow](task-03-release-workflow.md) — release.yml (all Node-20 pins)
- [x] [task-04-review-misc-workflows](task-04-review-misc-workflows.md) — claude.yml, claude-code-review.yml, opencode-code-review.yml, notify-docs.yml, preview-env.yml, pr-title.yml, plugin-registry-index.yml

Wave 2:
- [x] [task-05-verify](task-05-verify.md) — runtime audit + lint + contract tests + YAML sanity
```

## Tests

No product behavior is testable — the changed artifacts are workflow YAML only. Pre-PR validation:

- **Runtime audit:** rerun the procedure in [audit.md](audit.md) (fetch each pinned `action.yml`, assert `runs.using` is `node24` or `composite`) and confirm zero `node20` pins remain.
- **Pinning lint:** `python3 .github/scripts/lint-action-pinning.py` and `python3 .github/scripts/lint-action-pinning_test.py` still pass (every new ref is a 40-char SHA).
- **Workflow contract tests:** `python3 .github/scripts/release-workflow-contract_test.py`, `claude-code-review-workflow-contract_test.py`, `preview-env-workflow-contract_test.py` — the two SHA-asserting contract tests only pin `crazy-max/ghaction-import-gpg@2dc316d…` and `actions/setup-node@48b55a0…`, which are unchanged.
- **YAML sanity:** `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]"` over every changed workflow.
- **Runtime smoke (post-PR):** watch the first `backend-tests`/`e2e-tests`/`frontend-tests` run for the Node-20 deprecation warning; confirm it is gone.

## Verification Results

Complete 2026-08-07. All five tasks done; every task's `## Results` records the exact commands and outcomes.

- Applied 84 `uses:` replacements across 14 workflow files (checkout v4→v6, upload/download-artifact v4/v5→v7.0.1/v8.0.1, cache v4→v6.1.0, github-script v7→v9.0.0, docker login/buildx/build-push/metadata v3/v5/v6→v4.6.0/v4.2.0/v7.3.0/v6.2.0, gh-release v2→v3.0.2, wrangler v3→v4.0.0, semantic-pull-request v5→v6.1.1, Pages trio → v6.0.0/v5.0.0/v5.0.0, go-test-action v0→v1.1.0).
- Final runtime audit (`/tmp/opencode/audit_final.py`): 25 distinct pinned actions; every `runs.using` is `node24` or `composite` — **zero `node20` pins remain**. The three annotated-tag-object pins (rust-cache, claude-code-action, pnpm/action-setup) were dereferenced to their commits and re-verified (node24 / composite / node24).
- `python3 .github/scripts/lint-action-pinning.py` → "✓ All 18 workflow file(s) use SHA-pinned action refs."; `lint-action-pinning_test.py` → 9 OK.
- Contract tests: `release-workflow-contract_test.py` → 24 OK, `claude-code-review-workflow-contract_test.py` → 9 OK, `preview-env-workflow-contract_test.py` → 1 OK.
- YAML parse: 18/18 workflows OK; `git diff --check` clean.
- Post-merge smoke: pending — first `backend-tests`/`e2e-tests`/`frontend-tests` run after merge should no longer show the Node-20 deprecation warning.

## PR Fixup (2026-08-07)

PR #2401 (`ci: upgrade GitHub Actions to Node 24 runtime`):

- CodeRabbit/zizmor flagged the two v4→v6 checkout blocks in the arm64 release jobs (`docker-arm64`, `docker-universal-arm64`) for missing `persist-credentials: false` (artipacked — the checked-out tree feeds the Docker build context). Fixed in `b7624fd` (`ci: disable checkout credential persistence in arm64 release jobs`), scoped to exactly the two changed blocks; the other release.yml checkouts already set the flag.
- Final head `b7624fd32e43828c0ff50b90e73013fda3bf774e`: 47 checks passed, 0 failed, 0 pending, `mergeable_state: clean`, unresolved threads 0, hidden threads 0, actionable issue comments 0.
- Post-merge smoke still pending as above.

## Open Questions

None — target SHAs and compatibility are resolved in the table above.
