---
id: "03-consumer-docs"
title: "Prove consumer compatibility and document automation reads"
status: done
wave: 3
depends_on: ["02-host-reader"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AUTOMATION-READS-001
acceptance_criteria:
  - AC-PLUGINS-AUTOMATION-READS-001.4
  - AC-PLUGINS-AUTOMATION-READS-001.5
  - AC-PLUGINS-AUTOMATION-READS-001.6
  - AC-PLUGINS-AUTOMATION-READS-001.8
  - AC-PLUGINS-AUTOMATION-READS-001.11
  - AC-PLUGINS-AUTOMATION-READS-001.12
system_design:
  - ../../specs/plugins/system-design/automation-history-reads.md
---

# Task 03: Prove Consumer Compatibility and Document Automation Reads

## Summary

Prove that an independent public-SDK consumer can reconcile observations and
handle an older Host explicitly. Publish author guidance for the implemented
contract and its limits.

## In scope

- Independently compiled consumer fixture with public SDK imports only.
- Current-Host list/refresh flow after missed events, snapshot expiry,
  oversized-query handling, and permission revocation.
- Older Host with absent methods and an unchanged legacy call.
- Authoring and manifest resource documentation plus the maintained wire
  contract reference.
- Accurate release handoff without inventing a published SDK or Host version.

## Out of scope

- Implementing an external reporting, memory, calendar, or audit plugin.
- SDK publication, release version changes, deployment, and stable backports.
- A production reconciliation worker or automatic history partitioning.

## Acceptance

1. An independent consumer compiles using the candidate public SDK, discovers
   support and authority, and refreshes a run after missed completion events.
2. An older Host reports the new operations as unsupported while an existing
   supported call succeeds. Permission and paging errors remain explicit.
3. Public examples and resource/wire references match the implemented contract,
   including bounded completeness, source retention, and status meanings.

## Verification

Run from the repository root after the TDD implementation:

```bash
go -C pluginsdk test -trimpath -race ./... -run '^TestAutomationHistory' -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

`TestAutomationHistoryIndependentConsumer` must compile and exercise a separate
consumer module. A temporary module replacement can select the candidate SDK.
Record that substitution as local candidate evidence. It is not registry
installation or published-release evidence.

`TestAutomationHistoryOldHost` must use an older method surface. An optional
helper returning a wrapper is not proof that a remote Host supports the API.
The test must observe `Unimplemented`, avoid private fallbacks, and then make
an existing supported call successfully.

## Files likely touched

- `pluginsdk/automation_history_consumer_test.go` (new harness).
- `testdata/pluginsdk/automation-history-consumer/` (new public consumer
  fixture, including its own module file).
- `docs/public/plugins-authoring.md` reader table and focused API reference.
- `docs/public/plugins-manifest.md` automation read resource semantics.
- `docs/plans/plugins/GRPC-CONTRACT.md` optional SDK and wire reference.
- This package's requirement/design lifecycle and work-order results after
  all required implementation checks pass.

## Dependencies

- [Task 02](task-02-host-reader.md).

## Risks

- A package-local stub can hide public import or wire compatibility problems.
- Empty success on an older Host looks like an empty workspace and loses data.
- Public docs can overstate completeness beyond source retention or snapshot
  limits. The supported recovery path must preserve incomplete results.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/automation-history-reads.md).
- [Compatibility design](../../specs/plugins/system-design/automation-history-reads.md#compatibility-and-failures).
- [SDK distribution requirements](../../specs/plugins/requirements/sdk-distribution.md).
- `/tdd`, `/docs-maintainer`, and `docs/public/README.md`.

## Results

Status: done.

Added an independently compiled consumer module under
`testdata/pluginsdk/automation-history-consumer/` that uses only public SDK
imports. It exercises capability discovery, definition and run snapshots, exact
refresh after a missed event, revocation, oversized results, and an older Host
returning `Unimplemented` while an existing call still succeeds. Updated the
authoring guide, manifest resource vocabulary, and gRPC contract reference for
the implemented read contract, snapshot bounds, status semantics, and recovery
path.

Verification:

- `go -C pluginsdk test -trimpath -race ./... -run '^TestAutomationHistory' -count=1`: passed.
- Independent fixture module `go test -trimpath -race -count=1 ./...`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

The fixture uses a local module replacement for the candidate SDK. This is not
registry installation or published-release evidence.
