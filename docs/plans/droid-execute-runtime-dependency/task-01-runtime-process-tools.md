---
id: "01-runtime-process-tools"
title: "Supply and verify runtime process tools"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CONTAINER-TOOLS-001
acceptance_criteria:
  - AC-EXECUTORS-CONTAINER-TOOLS-001.1
  - AC-EXECUTORS-CONTAINER-TOOLS-001.2
  - AC-EXECUTORS-CONTAINER-TOOLS-001.3
system_design:
  - ../../specs/executors/system-design/container-runtime-tools.md
---

# Task 01: Supply and Verify Runtime Process Tools

## Summary

Add the missing runtime package and a smoke check against actual base and
universal images. Keep the production correction in the shared base dependency
list and preserve the existing executor and security contracts.

## In scope

- Create `scripts/test-docker-runtime.sh IMAGE [IMAGE ...]` before editing the
  Dockerfile. Check runtime identity and the exact parent-PID probe; use a real
  Python parent and child shells so parent equality is checked, not merely
  numeric output. Assert both zero and intentional nonzero command results.
- Exercise the seven-command sequence from the plan in order for twenty rounds,
  plus a three-second command with probes before and after its wait. Capture
  stdout/stderr and enforce per-command and overall timeouts. Use unique owned
  container names with `--rm`, `--network=none`, inherited entrypoints, and
  cleanup on failure. Do not mount host homes, repositories, or Docker sockets.
- Prove red on the unmodified images, then add `procps` to the root Dockerfile's
  final-stage apt list and run the same test against rebuilt images.
- Update the base-image inventory in `docs/public/docker.md` and
  `docs/images.md`. Use the docs-maintainer skill; the public Docker page is a
  how-to guide with reference disclosures. Keep additions factual and small.
- Update this work order and the plan with exact results and unrun live checks.

## Out of scope

- Vendoring Droid binaries or copying its supervisor into production or a
  permanent fixture; changing ACP, runtime permissions, credential handling,
  release workflows, image pins, or frontend code.
- Installing packages into running user containers, native hosts, or old images.
- Issue comments, issue closure, release publication, or provider logins.

## Acceptance

1. The runtime smoke script fails on the original images for the missing probe
   executable and passes on the rebuilt base and its universal derivative.
2. `procps` is declared once in the shared base runtime dependency list;
   UID 1000, entrypoints, permissions, and credential boundaries are preserved.
3. The image inventories and verification record match delivered behavior;
   simulated shell coverage is distinguished from authenticated Droid coverage.

## Verification

Use `/tdd` after the explicit implementation request. Mark this task
`in_progress`, create the smoke script, and run the red checks before changing
the dependency. The Dockerfile is configuration, but a real failing image
probe is available and is the regression gate for this repair.

From the repository root, after writing the test script, pull the immutable
inputs. The smoke inspects local images before running them; registry references
must be pulled and local tags must be built beforehand.

```bash
bash -n scripts/test-docker-runtime.sh
docker pull ghcr.io/kdlbs/kandev@sha256:33c9211aea048af542fb68da80088dc28ab3ed947378dd12f97d98e7d2d2bf93
docker pull ghcr.io/kdlbs/kandev@sha256:5a7558e4ed370652a8128b49d8d8701cd0639ba83b6648ca139e103d06c450de
# Both calls must fail specifically because ps is unavailable.
bash scripts/test-docker-runtime.sh ghcr.io/kdlbs/kandev@sha256:33c9211aea048af542fb68da80088dc28ab3ed947378dd12f97d98e7d2d2bf93
bash scripts/test-docker-runtime.sh ghcr.io/kdlbs/kandev@sha256:5a7558e4ed370652a8128b49d8d8701cd0639ba83b6648ca139e103d06c450de
```

After the one-package correction, build the real root Dockerfile and universal
recipe. This bounded packaging check reuses real published application binaries
because the application is not changing. It does not pretend to compile main.
The complete block runs in a subshell from the repository root and removes
only its own context, stopped bundle container, volume, and local image tags:

```bash
(
  set -euo pipefail
  droid_check_dir=$(mktemp -d /tmp/kandev-droid-image.XXXXXX)
  droid_check_id="droid-image-$(basename "$droid_check_dir")"
  droid_bundle_container="$droid_check_id-bundle"
  droid_base_tag="kandev-droid-check:$droid_check_id-base"
  droid_universal_tag="kandev-droid-check:$droid_check_id-universal"
  cleanup() {
    docker rm -v "$droid_bundle_container" >/dev/null 2>&1 || true
    docker image rm "$droid_universal_tag" "$droid_base_tag" >/dev/null 2>&1 || true
    rm -r "$droid_check_dir"
  }
  trap cleanup EXIT
  mkdir -p "$droid_check_dir/bundle/bin"
  docker create --name "$droid_bundle_container" \
    ghcr.io/kdlbs/kandev@sha256:33c9211aea048af542fb68da80088dc28ab3ed947378dd12f97d98e7d2d2bf93 >/dev/null
  docker cp "$droid_bundle_container:/app/apps/backend/bin/." "$droid_check_dir/bundle/bin/"
  docker cp "$droid_bundle_container:/usr/local/bin/agentctl" "$droid_check_dir/bundle/bin/agentctl"
  cp docker-entrypoint.sh "$droid_check_dir/docker-entrypoint.sh"
  docker build --platform linux/amd64 -f Dockerfile -t "$droid_base_tag" "$droid_check_dir"
  docker build --platform linux/amd64 -f Dockerfile.universal \
    --build-arg "BASE_IMAGE=$droid_base_tag" -t "$droid_universal_tag" .
  bash scripts/test-docker-runtime.sh "$droid_base_tag" "$droid_universal_tag"
)
```

Record built image IDs and actual probe/command results before cleanup. Repeat
on a native arm64 runner with its matching published bundle if available;
otherwise report arm64 as unrun. Do not change the host's binfmt configuration.

Run the documentation and diff gates from the root:

These commands require Python 3 and Node.js 24 on the verification runner.
When host Node is unavailable, use the existing CI runtime container with a
read-only checkout and no networking; do not install host tooling just for the
documentation checks.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Run the repository's `.github/scripts/pr-docs.cjs` `validateCoverage` preflight
against this work order, its linked plan/design/requirement, and changed paths.
Do not publish a GitHub status or PR as part of that local validation.

### Credential-dependent live smoke

Only when test credentials are explicitly supplied through an authorized
disposable environment, run the actual installed Droid binary with this prompt:

```bash
droid --version
droid exec --auto low 'Use Execute in the foreground. Run pwd, then ls -la, then cd /tmp, then printf "execute-test\n", then date, then id, then uname -srm. Run one command per Execute call, in that order. Do not use fireAndForget. Report stdout, stderr, and exit status for each command.'
```

Repeat with 0.218.0 and the currently supported installed version where
available; record exact versions and image IDs. No individual Execute result
may contain SIGKILL. CLI exit 0 alone is insufficient. An auth error or absence
of authorized credentials is an explicit unrun live check, not a passing test
and not a reason to inspect another installation's secrets. The credential-free
image gate and confirmed supervisor reproduction remain required evidence.

## Files likely touched

- `Dockerfile`
- `scripts/test-docker-runtime.sh` (new)
- `docs/public/docker.md`
- `docs/images.md`
- This work order and `plan.md` for results; the linked draft specification
  pair for accepted/current status after implementation is requested.

## Dependencies

None. One sequential work order; no delegation is authorized.

## Risks

The smoke script validates the missing prerequisite rather than replacing
Droid's implementation. A policy-denied or shadowed `ps` remains outside this
fix. Apt/toolchain network failures must be distinguished from regression
failures; do not relax container security to make a test pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/container-runtime-tools.md)
- [System design](../../specs/executors/system-design/container-runtime-tools.md)
- [Diagnosis and immutable inputs](plan.md#confirmed-diagnosis)
- `Dockerfile`, `Dockerfile.universal`, `docker-entrypoint.sh`
- `scripts/test-kubernetes-worker-images.sh` for owned-resource smoke patterns

## Results

Completed on 2026-09-29. The production correction is one runtime package in
`Dockerfile`; universal inherits it without a recipe change. Public inventories
in `docs/public/docker.md` and `docs/images.md` now identify that tool.

| Check | Result |
| --- | --- |
| `bash -n scripts/test-docker-runtime.sh` and embedded Python syntax | Passed |
| Smoke on the original base and universal digests above | Both failed before the dependency edit with parent-probe exit 127 and `ps: command not found` |
| Documented pull-then-smoke sequence after removing the owned original image references | Both pulls restored the absent references and the smokes reached the expected missing-`ps` failure; rebuilt local base and universal tags still passed |
| `docker build --platform linux/amd64 -f Dockerfile -t kandev-3617-implementation:base /tmp/kandev-3617-implementation/ctx` | Passed using the documented published-bundle context |
| `docker build --platform linux/amd64 -f Dockerfile.universal --build-arg BASE_IMAGE=kandev-3617-implementation:base -t kandev-3617-implementation:universal .` | Passed; an interrupted initial attempt was restarted after confirming no build client remained |
| `bash scripts/test-docker-runtime.sh kandev-3617-implementation:base kandev-3617-implementation:universal` | Passed: both UID 1000, parent probes, 140 ordered commands each, delayed probes, stderr and exit status |
| `python3 scripts/list-docs.py validate` | Passed |
| `python3 scripts/lint-spec-files.test.py` | 36 tests passed |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| `node --test scripts/validate-public-docs.test.mjs` | 62 tests passed |
| `node scripts/validate-public-docs.mjs` | 47 published pages validated |
| `.github/scripts/pr-docs.cjs` local `validateCoverage` preflight | Covered, no errors |
| `git diff --check` and changed/untracked file inventory | Passed |

Built base image ID:
`sha256:646ebf548c36729b1da370d32ca0b6f7ad504424eb6a7600fa00c769b1a5e0d1`.
Built universal image ID:
`sha256:000337855f5556fffe7573cbcf13824f54f2fe42115abcfa844d97152f73bf19`.

No authenticated Droid/ACP session or arm64 image execution was performed.
The design-phase unmodified-supervisor A/B reproduction supplies causal
evidence, while the permanent smoke checks the runtime prerequisite and shell
behavior. Application binaries were reused, not recompiled. No credentials,
permission changes or live Kandev volumes were used.

PR review clarified the local-image prerequisite in the script's usage comments
and added the explicit pulls above. The repeat checks passed without changing
the smoke's behavior or requiring a registry copy of locally built images.
