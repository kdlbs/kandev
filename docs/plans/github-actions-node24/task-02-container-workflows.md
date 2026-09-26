---
id: "02-container-workflows"
title: "Upgrade Node-20 docker actions in container workflows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
---

# Task 02: Upgrade Node-20 docker actions in container workflows

- **Acceptance:** every `uses:` pin in `universal-rebuild.yml` and `ci-base-image.yml` runs on Node 24; the docker v3/v6 pins (login, setup-buildx, build-push) move to the v4/v7 lines with Node 24 runtime.
- **Verification:**
  - `python3 .github/scripts/lint-action-pinning.py`
  - runtime audit per `audit.md` for the two files
  - `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/universal-rebuild.yml .github/workflows/ci-base-image.yml`
- **Files likely touched:**
  - `.github/workflows/universal-rebuild.yml` (setup-buildx:55 → v4.2.0; login:58 → v4.6.0; build-push:69 → v7.3.0)
  - `.github/workflows/ci-base-image.yml` (setup-buildx:40 → v4.2.0; login:51 → v4.6.0; build-push:102,120 → v7.3.0)
- **Dependencies:** None.
- **Parallelism:** sequential by default; parallel-safe if run alongside tasks 01, 03, 04 (all files disjoint).

## Changes

| File:line | From | To |
|---|---|---|
| universal-rebuild.yml:55 | `docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3` | `docker/setup-buildx-action@bb05f3f5519dd87d3ba754cc423b652a5edd6d2c # v4.2.0` |
| universal-rebuild.yml:58 | `docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3` | `docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0` |
| universal-rebuild.yml:69 | `docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8 # v6` | `docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a # v7.3.0` |
| ci-base-image.yml:40 | `docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3` | `docker/setup-buildx-action@bb05f3f5519dd87d3ba754cc423b652a5edd6d2c # v4.2.0` |
| ci-base-image.yml:51 | `docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3` | `docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0` |
| ci-base-image.yml:102,120 | `docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8 # v6` | `docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a # v7.3.0` |

Line numbers are 2026-08-07 snapshots; match by `uses:` string if they moved.
Do not change `with:` blocks — none use inputs removed by docker v4/v7
(deprecated buildx inputs, `DOCKER_BUILD_NO_SUMMARY`).

## Inputs

plan.md target table + docker compatibility notes; `audit.md`.

## Output contract

Summary, files changed, exact commands and outcomes, audit output for the two files, task/plan status updates.

## Results

- Applied 2 setup-buildx, 2 login-action, 3 build-push replacements across universal-rebuild.yml / ci-base-image.yml.
- `python3 .github/scripts/lint-action-pinning.py` → all 18 workflows SHA-pinned.
- Runtime audit: all pins in the two files `node24`, zero `node20`.
- YAML parse OK; `git diff --check` clean. None.
