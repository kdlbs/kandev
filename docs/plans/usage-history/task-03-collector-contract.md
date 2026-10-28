---
id: "03-collector-contract"
title: "Normalize collector identities and indexed selection"
status: pending
wave: 3
depends_on: ["02-lifecycle-migration"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-004
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-002.1
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-003.1
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.3
  - AC-COSTS-USAGE-HISTORY-003.4
  - AC-COSTS-USAGE-HISTORY-003.5
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-004.1
  - AC-COSTS-USAGE-HISTORY-004.2
  - AC-COSTS-USAGE-HISTORY-004.3
  - AC-COSTS-USAGE-HISTORY-004.4
  - AC-COSTS-USAGE-HISTORY-004.5
  - AC-COSTS-USAGE-HISTORY-004.6
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 03: Normalize collector identities and indexed selection

## Summary

Expose a versioned measurement contract independent of collector tools.
Replace workspace-wide overlap loops with bounded identity/coverage matching and deterministic source selection.

## In scope

- Separate authenticated collector provenance, core-approved underlying work identity, provider, model, client, and serving platform.
- Add subject-binding admission and authorized retained corrections after live task deletion.
- Preserve nullable fields, cost/token semantics, integer precision, source versions, and revision/digest concurrency results.
- Support explicit same-source supersession for model/provider corrections. Missing partial-report entries never imply deletion or zero.
- Add immutable snapshot/keyset collection-target discovery, including concurrent addition/deletion/resume tests.
- Index scope/work/model/provider/semantic partitions and coverage intervals. Use ordered matching inside affected partitions.
- Resolve proven work equivalence before dimension partitions become additive, including unknown-to-known provider/model/semantic transitions.
- Use immutable observation item IDs or explicit complete-set supersession/tombstones. Invalidate old and new partitions after dimension corrections.
- Select tokens and cost independently only for equivalent coverage. Preserve ambiguous overlap and separate lifetime/day representations.
- Keep existing DTOs working through a compatibility mapper. Regenerate protobufs and update SDK/Host documentation together.
- Add a replacement-collector fixture without integrating another real external tool.

## Out of scope

- Plugin process changes, global query aggregation, summary rebuilds, and arbitrary source-priority claims from plugins.

## Acceptance

- Different collectors reporting the same proven work yield one contribution. Unproven aliases remain incomplete and do not silently merge.
- Concurrent first inserts and later corrections return accurate accepted rows. Stale/conflicting writes cannot roll back a newer value.
- Forged tenant/actor/binding claims fail, including tombstoned work. Older supported clients keep their wire behavior.

## Verification

```bash
make -C apps/backend proto
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/plugins/... ./pkg/pluginsdk/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/plugins/... -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/{models/usage.go,repository/interface.go,service/service.go}`
- `apps/backend/internal/analytics/repository/sqlite/usage.go` and new selection/index tests
- `apps/backend/internal/plugins/{host_data.go,host_data_queries.go}` and Host contract tests
- `apps/backend/pkg/pluginsdk/{usage.go,host_usage.go,host.go}`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto` and its existing generated output
- `docs/public/plugins-authoring.md`, `docs/plans/plugins/GRPC-CONTRACT.md`

## Dependencies

Task 02. Retained bindings and lifecycle authorization are prerequisites for post-deletion corrections.

## Risks

Different tools can name one provider thread differently. Unsupported aliases must remain unknown rather than guessed.
Do not use model prefixes or collector IDs as proof of underlying work identity.
Contract additions must not change the existing proto numbers or numeric semantics.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-002/003/004 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Measurement contract, Source identity and indexed selection.
- Existing capability gates, revision tests, native observation identities, and [Host ADR](../../decisions/0043-plugin-host-data-api.md).

## Results

Pending.
