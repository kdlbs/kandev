---
id: "02-remove-pin-automation"
title: "Remove pin automation and update documentation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
role: implementer
model_tier: default
---

# Task 02: Remove pin automation and update documentation

## Acceptance

- Every workflow, script, test, plan, and ADR that exists only for PR 1950's
  scheduled pin updates is removed, and the unrelated GitLab test change is
  restored to `origin/main` behavior without destructive git commands.
- Runtime inventory, README, Codex ACP decision, and public agent documentation
  describe unversioned managed runtimes plus the explicit host Settings update.
- Public documentation validation passes and no obsolete scheduled-pin
  references remain.

## Verification

- `python3 .github/scripts/lint-action-pinning_test.py`
- `python3 .github/scripts/lint-action-pinning.py`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`
- `rg -n "scheduled-core-agent-version-pins|update_agent_versions|claudeACPVersion|codexACPVersion|opencodeACPVersion|copilotACPVersion|geminiVersion" . --glob '!docs/plans/managed-agent-runtime-updates/**'`

## Files likely touched

- `.github/workflows/lint-action-pinning.yml`
- `.github/workflows/update-agent-versions.yml` (delete)
- `.github/scripts/update-agent-versions-workflow_test.py` (delete)
- `scripts/update_agent_versions.py` (delete)
- `scripts/update_agent_versions_test.py` (delete)
- `docs/plans/agent-version-updates/**` (delete)
- `apps/backend/internal/gitlab/poller_health_test.go`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `README.md`
- `docs/decisions/0034-agentclientprotocol-codex-acp.md`
- `docs/public/agents-and-profiles.md`

## Dependencies

None. Do not edit the five agent Go implementations or their tests; Task 01
owns them in the parallel wave.

## Inputs

- Spec `Why`, `What`, and `Out of scope`
- ADR `Decision`, `Consequences`, and rejected pinning alternative
- Plan section `Documentation and PR replacement`
- Current branch diff against `origin/main`

## Output contract

Report intent/acceptance, base/head SHA, deleted/restored/documented files,
spec/ADR sections, risk tags (`pr-replacement`, `docs`, `ci-cleanup`), exact
validation results, and uncertainties. Update only this task file to `done`;
do not edit `plan.md`.
