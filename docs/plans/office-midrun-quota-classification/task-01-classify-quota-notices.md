---
id: "01-classify-quota-notices"
title: "Classify quota notices and prove provider recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
  - REQ-AGENTS-CODEX-USAGE-LIMIT-001
acceptance_criteria:
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.6
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.9
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.10
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.11
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.5
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.6
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.7
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
  - ../../specs/agents/system-design/codex-usage-limit-classification.md
---

# Task 01: Classify Quota Notices and Prove Provider Recovery

## Summary

Recognize the complete Claude five-hour and usage-limit notices, plus Codex's workspace-credit notice.
Reuse existing reset parsing and prompt correlation so configured routing handles recognized quota failures before terminal agent accounting.

## In scope

- Three provider-specific, fence-aware signatures and their hard-quota precedence over HTTP 429.
- Five-hour clock inference through the existing parser and observation timestamp.
- Classifier, ACP replay, dynamic circuit, and real Office routing regressions from the plan's test matrix.
- Exact documentation and changed-Go lint checks below.
- A targeted quota-regression step in the existing Windows native CI cohort.

## Out of scope

- A generic `Internal error` fallback rule or uncorrelated stderr extraction.
- New Office, adapter, or orchestrator production behavior beyond the catalogue and reset gate.
- New policy, automatic recovery of already-paused agents, UI, copy, schema, flags, or configuration.
- Manual-recovery predicate changes and the issue's separate PAT/npm observations.

## Acceptance

1. All three complete notices produce high-confidence hard quota classifications with existing flags and precedence. Only eligible five-hour wording gains clock-only inference.
2. Codex attaches a credit notice only to its current prompt's matching generic error. Bare errors, other prompts, success, cancellation, and specific errors retain existing handling.
3. Real Office quota events degrade and skip the failed route without changing agent failure counts. Dynamic routing preserves shared-binding exclusion and exclusive reset probes.

## TDD sequence

1. Add `provider_quota_notices_test.go` with the four classifier tests named in the plan.
2. Use fixed observation `2026-11-09T09:10:00Z` for the reported Berlin reset fixture.
3. Cover both post-start phases, apostrophe and spacing variants, optional labels, and HTTP 429 alongside each new signature.
4. Cover reset precedence, invalid zones, delayed classification, generic usage clocks, and reset provenance from only the matching unfenced diagnostic line. Include weekly, rate, provider, fence, prose, other-channel, quoted dated text, and split-line compatibility.
5. Add the two ACP tests with the existing `replayFixtureThroughAdapter` harness.
6. Cover notice plus generic failure, absent notice, stale prompt generation, another session, cancellation, successful completion, and specific terminal errors.
7. Add Office integration regressions with `newTestServiceWithBus`, `buildRoutedScheduler`, and the existing routed launcher helpers.
8. Cover task-backed and taskless failures through actual lifecycle events, with Claude-to-Codex and Codex-to-Claude orders.
9. Assert stored quota code, degraded health, reset or backoff deadline, requeued run, selected alternative, and stable Office identity.
10. Start the repeated-failure case one below the effective pause threshold. Drive three recognized failures with deterministic route-health setup between runs.
11. Assert the original counter and unpaused state after each handled failure. Preserve existing stale-event behavior.
12. Add `TestEngineRoutesProviderQuotaNotices` with a fixed clock, same-binding sibling, and healthy alternative.
13. Prove shared-circuit exclusion before reset, exclusive probe admission at reset, and closure only after success.
14. Run the red command below before production changes. The new positive classifier, ACP credit-correlation, and Office notice cases must fail for missing classification.
15. Add only the three rules, their HTTP 429 allowlist entries, and the exact five-hour clock gate.
16. Add a targeted step to the Windows native CI cohort with the exact command below.
17. Run the affected package checks, formatting, lint, and documentation checks. Record actual results here and in the plan.

Use `mustRuleOutsideMarkdownFences` for the new signatures.
Use horizontal whitespace rather than `\s` within their diagnostic phrases.
Keep existing credit/session rules first and new quota rules ahead of rate rules.
Reuse `Classify`, `parseResetClockHintAt`, and the existing invariants without adding a recovery policy.

## Verification

Run these blocks from the repository root.
The first block requires the permanent tests added in the TDD sequence.

Red regression command:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/routingerr ./internal/agentctl/server/adapter/transport/acp ./internal/office/service -run 'TestClassifyClaudeFiveHourLimit|TestClassifyClaudeUsageLimit|TestClassifyCodexWorkspaceCredits|TestCodexWorkspaceCreditsProjectsMatchingGenericPromptError|TestOfficeProviderQuota' -count=1)
```

Green and compatibility commands:

```bash
gofmt -w apps/backend/internal/agent/runtime/routingerr/rules.go apps/backend/internal/agent/runtime/routingerr/routingerr.go apps/backend/internal/agent/runtime/routingerr/provider_quota_notices_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/codex_workspace_credits_test.go apps/backend/internal/agent/runtime/dynamic/engine_provider_quota_test.go apps/backend/internal/office/service/provider_quota_routing_test.go
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic ./internal/agentctl/server/adapter/transport/acp ./internal/office/scheduler ./internal/office/service -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/routingerr/rules.go apps/backend/internal/agent/runtime/routingerr/routingerr.go apps/backend/internal/agent/runtime/routingerr/provider_quota_notices_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/codex_workspace_credits_test.go apps/backend/internal/agent/runtime/dynamic/engine_provider_quota_test.go apps/backend/internal/office/service/provider_quota_routing_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/office-midrun-quota-classification
```

PR-documentation coverage preflight:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/agents/requirements/claude-session-limit-classification.md',
  'docs/specs/agents/system-design/claude-session-limit-classification.md',
  'docs/specs/agents/requirements/codex-usage-limit-classification.md',
  'docs/specs/agents/system-design/codex-usage-limit-classification.md',
  'docs/plans/office-midrun-quota-classification/plan.md',
  'docs/plans/office-midrun-quota-classification/task-01-classify-quota-notices.md',
  'docs/plans/claude-session-limit-classification/plan.md',
  'docs/plans/codex-usage-limit-classification/plan.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/agent/runtime/routingerr/rules.go', status: 'modified' },
    { filename: 'apps/backend/internal/agent/runtime/routingerr/routingerr.go', status: 'modified' },
    { filename: '.github/workflows/backend-tests.yml', status: 'modified' },
    ...paths.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify({ status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

The preflight models the prospective runtime change and publishes no GitHub status.

Native Windows command, from `apps/backend` in the existing native CI cohort:

```powershell
$env:GOMAXPROCS = '2'
$env:GOMEMLIMIT = '2GiB'
go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=5m ./internal/agent/runtime/routingerr ./internal/agentctl/server/adapter/transport/acp -run '^(TestClassifyClaudeFiveHourLimit.*|TestClassifyClaudeUsageLimit|TestClassifyCodexWorkspaceCredits|TestProviderQuotaNoticeCollisionsAndPrecedence|TestCodexWorkspaceCreditsProjectsMatchingGenericPromptError|TestCodexWorkspaceCreditsPreservesPromptIsolation)$'
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

Give the new step a ten-minute timeout and preserve existing job permissions.
If this executor cannot run Windows tests, record that limitation and the hosted CI result before delivery.

## Files likely touched

Production:

- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`

CI configuration:

- `.github/workflows/backend-tests.yml`

Permanent regressions:

- `apps/backend/internal/agent/runtime/routingerr/provider_quota_notices_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/codex_workspace_credits_test.go`
- `apps/backend/internal/agent/runtime/dynamic/engine_provider_quota_test.go`
- `apps/backend/internal/office/service/provider_quota_routing_test.go`

Keep new tests separate from large existing suites.
No production changes in Office or ACP transport are expected.

## Dependencies

None. The existing parser, Codex correlation, and Office fallback implementation are present on the investigated checkout.

## Risks

- A log-only Codex notice cannot establish same-prompt evidence. Preserve generic-error handling in that case.
- Generic usage wording does not prove a renewal period. Its clock must not become a guessed deadline.
- Candidate health becoming probe-eligible does not establish recovery. Preserve exclusive probes and success checks.
- Do not let diagnostic-only messages bypass existing output/effect and cancellation fences.

## Parallelism

`sequential`

## Inputs

- The two requirement documents and criteria listed in frontmatter.
- The two system designs listed in frontmatter, especially classification and consumer integration.
- The [plan](plan.md), including fixture provenance, evidence limits, and the complete compatibility matrix.
- `routingerr/claude_weekly_limit_test.go`, `routingerr/structured_test.go`, and `routingerr/resethint_test.go`.
- `transport/acp/dialect_codex_usage_test.go` and `provider_diagnostic_marker_test.go`.
- `dynamic/engine_test.go`, including existing quota and reset-probe tests.
- `office/service/taskless_routed_launch_test.go` and `office/scheduler/routing_lifecycle_test.go`.
- Scoped backend and agentctl instructions, plus `.agents/skills/tdd/references/backend-tests.md`.
- `.github/AGENTS.md` and the existing Windows native cohort's bounded test-step patterns.

## Results

Completed on 2026-11-10.

- Added the three provider-scoped rules, their HTTP 429 allowlist entries, and the exact five-hour clock gate.
- Text-derived reset hints for the new rules now come only from the matching unfenced diagnostic line; the dated parser is scoped the same way. Fenced, prose, other-channel, and quoted dated text before the genuine notice cannot supply a deadline.
- Added `provider_quota_notices_test.go`, `codex_workspace_credits_test.go`, `engine_provider_quota_test.go`, and `provider_quota_routing_test.go`.
- Classifier coverage includes both post-start phases, apostrophe and spacing variants, optional labels, fenced and mid-sentence negatives, split-line rejection, structured status precedence, and the fixed `2026-11-09T09:10:00Z` Berlin reset producing `2026-11-09T11:50:00Z`.
- ACP coverage includes matching generic errors, absent notices, stale generations, user/fenced/quoted chunks, cancellation, successful completion, and specific terminal errors.
- Dynamic coverage proves shared-binding exclusion, a healthy alternative, exclusive reset probing, and closure only after probe success.
- Office coverage drives real lifecycle events for task-backed and taskless Claude-to-Codex and Codex-to-Claude routes, asserting health degradation, quota attempt code, requeue, next launch, and unchanged failure counts one below the pause threshold.
- Verification passed: affected package tests, changed-Go lint, documentation validation/lint, workflow contract/pinning checks, PR coverage preflight, and `git diff --check`.
- The Windows native step selects all `TestClassifyClaudeFiveHourLimit*` reset-provenance regressions. It was added but not executed on this Linux host; hosted CI owns that evidence. The original-source probes and disposable candidate results above remain investigation evidence only.
