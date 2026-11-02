---
id: "01-sdk-runtime"
title: "Migrate the plugin SDK and evaluate pinned Nono 0.81.0"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-012
  - REQ-PLUGINS-SANDBOX-002
acceptance_criteria:
  - AC-AGENTS-SANDBOX-012.1
  - AC-AGENTS-SANDBOX-012.3
  - AC-AGENTS-SANDBOX-012.4
  - AC-PLUGINS-SANDBOX-002.4
system_design:
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
  - ../../specs/plugins/system-design/sandbox-provider-v2.md
---

# Task 01: Migrate the plugin SDK and evaluate pinned Nono 0.81.0

## Summary

Migrate the dedicated plugin consumer without a backend module dependency. Evaluate the explicitly pinned runtime candidate before changing capability claims.

## In scope

- Plugin imports, go.mod/go.sum, CI SDK pin, packaging tool separation, README and runtime-lock/license records.
- Retain baseline behavior while testing 0.81.0; add explicit request/response and negative-enforcement probes for later slices. Record source/archive/binary digests and changed behavior.

## Out of scope

- No new approval kind enabled, no host rollout, no global install or retained-instance access.

## Acceptance

- Independent public SDK consumer builds and package validation pass; local replace is development-only.
- The pinned 0.81.0 artifact passes baseline conformance or produces a specific blocker. Historical 0.80.0 receipts remain unchanged.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd ../kandev-plugin-nono-main && make validate verify-package)
(cd pluginsdk && go test -race ./...)
(cd ../kandev-plugin-nono-main && sh scripts/check-sdk-consumer.sh)
: "${NONO_ACCEPTANCE_BINARY:?Set the exact verified candidate binary path}"
python3 scripts/check-nono-real.py --check-prerequisites
python3 scripts/check-nono-real.py --conformance --record /tmp/nono-expansion-conformance.json
```

## Files likely touched

- `../kandev-plugin-nono-main/go.mod`
- `../kandev-plugin-nono-main/server/`
- `../kandev-plugin-nono-main/Makefile`
- `../kandev-plugin-nono-main/runtime-lock.json`
- `../kandev-plugin-nono-main/.kandev-sdk-ref`
- `../kandev-plugin-nono-main/.github/workflows/`
- `../kandev-plugin-nono-main/scripts/check-sdk-consumer.sh (new)`
- `scripts/check-nono-real.py`

## Dependencies

None.

## Risks

A runtime release can change confinement, proxy behavior, or fixture prerequisites. A successful source build alone cannot approve the upgrade.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Completed 2026-11-02. Public SDK pin: `v0.0.0-20261028184252-29170265fc88`, resolved independently without any local replacement. SDK imports migrated and dependency files tidied. Packaging uses a separately identified host tool checkout.

- RED: `TestDescribePinsPackagedLinuxRuntime` failed on the expected 0.81.0 version assertion before the runtime pin changed (`/tmp/nono-081-red.log`).
- GREEN: `NONO_ACCEPTANCE_BINARY=/tmp/nono-expansion-runtime-081/nono make validate verify-package` in the plugin repository passed format, vet, race tests with actual runtime, runtime packaging tests, release identity tests, and archive verification.
- `sh scripts/check-sdk-consumer.sh` passed from an isolated temporary consumer with a public module dependency and no sibling host checkout.
- `(cd pluginsdk && GOMAXPROCS=4 go test -trimpath -p=4 -race ./...)` passed.
- Explicit candidate prerequisite and conformance checks passed. The runtime suite records all ten positive/negative/PTY/approval/cleanup checks in `/tmp/nono-expansion-conformance.json`.
- `NONO_ACCEPTANCE_BINARY=/tmp/nono-expansion-runtime-081/nono python3 scripts/check-nono-real.test.py`: five tests passed, including real conformance. `python3 scripts/nono_managed.test.py`: six safety tests passed. This does not run managed product acceptance.
- The packaged Apache license matches the 0.81.0 source license byte-for-byte.

Candidate source: `b2d859deeb7a65f9ca5b334eb8726637ba76c866`. Archive SHA-256: `d395bc5fd320e3c6be5fc30aafc61c9fee11fed0a087a2175204ce845eadcdd5`, matched to the published checksum. Executable SHA-256: `2d3c29641c87921f7cd469387e4838b1676d54edb7291aebda7a780787d7c1b2`.

No new approval capability is enabled. Fresh current-pair managed acceptance remains Task 10. No production plugin release, retained instance, or main :9998 mutation occurred.
