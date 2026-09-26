---
id: "03-release-workflow"
title: "Upgrade Node-20 actions in release.yml"
status: done
wave: 1
depends_on: []
plan: "plan.md"
---

# Task 03: Upgrade Node-20 actions in release.yml

- **Acceptance:** every `uses:` pin in `release.yml` runs on Node 24; the release workflow contract test still passes.
- **Verification:**
  - `python3 .github/scripts/lint-action-pinning.py`
  - `python3 .github/scripts/release-workflow-contract_test.py`
  - runtime audit per `audit.md` for release.yml
  - `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/release.yml`
- **Files likely touched:** `.github/workflows/release.yml` only.
- **Dependencies:** None.
- **Parallelism:** sequential by default; parallel-safe if run alongside tasks 01, 02, 04 (all files disjoint).

## Changes

| File:line | From | To |
|---|---|---|
| release.yml:103,612,677,741,877,1412,1621,1803,1975,2017,2070 | `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6` | unchanged (already node24) |
| release.yml:1491,1681 | `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4` | `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6` |
| release.yml:697,826,1253,1380 | `actions/upload-artifact@330a01c490aca151604b8cf639adc76d48f6c5d4 # v5` | `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1` |
| release.yml:1416,1495 | `actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4` | `actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1` |
| release.yml:749,959,1873,1879,2037 | `actions/download-artifact@634f93cb2916e3fdff6788551b99b062d0335ce0 # v5` | `actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1` |
| release.yml:1433,1510,1569,1688,1743 | `docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3` | `docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0` |
| release.yml:1430,1507,1566,1625,1685,1740 | `docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3` | `docker/setup-buildx-action@bb05f3f5519dd87d3ba754cc423b652a5edd6d2c # v4.2.0` |
| release.yml:1452,1524,1642,1696 | `docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8 # v6` | `docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a # v7.3.0` |
| release.yml:1446,1518,1557,1636 | `docker/metadata-action@c299e40c65443455700f0fdfc63efafe5b349051 # v5` | `docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0` |
| release.yml:1940 | `softprops/action-gh-release@3bb12739c298aeb8a4eeaf626c5b8d85266b0e65 # v2` | `softprops/action-gh-release@3d0d9888cb7fd7b750713d6e236d1fcb99157228 # v3.0.2` |

Line numbers are 2026-08-07 snapshots; match by `uses:` string if they moved.
The `# v6` checkout rows are listed for completeness — they are already
node24 and require no edit. Do not change `with:` blocks; inputs used
(`files:`, `body_path:`, `tag_name:`, `tags: type=raw,…`) are all preserved
in the target versions. The contract test pins
`crazy-max/ghaction-import-gpg@2dc316d…` and `actions/setup-node@48b55a0…` —
both unchanged.

## Inputs

plan.md target table + compatibility notes; `audit.md`.

## Output contract

Summary, files changed, exact commands and outcomes, audit output for release.yml, task/plan status updates.

## Results

- Applied 2 checkout v4→v6, 4 upload-artifact v5→v7.0.1, 7 download-artifact v4/v5→v8.0.1, 6 login v3→v4.6.0, 6 buildx v3→v4.2.0, 4 build-push v6→v7.3.0, 5 metadata v5→v6.2.0, 1 gh-release v2→v3.0.2 in release.yml (35 uses total).
- `python3 .github/scripts/lint-action-pinning.py` → all SHA-pinned.
- `python3 .github/scripts/release-workflow-contract_test.py` → 24 tests OK.
- Runtime audit: all pins `node24`, zero `node20`; YAML parse OK; `git diff --check` clean.
- `with:` blocks untouched (files/body_path/tag_name/tags inputs all preserved in targets). None.
