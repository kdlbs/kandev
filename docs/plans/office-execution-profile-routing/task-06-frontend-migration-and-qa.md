---
id: "06-frontend-migration-and-qa"
title: "Frontend, migration cleanup, and QA"
status: done
wave: 5
depends_on: ["05-cross-provider-continuation"]
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 06: Frontend, migration cleanup, and QA

**Acceptance:** routing settings select concrete profiles per provider/tier and show profile/provider/model/account summaries; agent surfaces retain stable Office identity and expose resolved execution profile/fallback; onboarding stores authoritative profile IDs; copy-on-select behavior is removed or migrated; desktop and mobile layouts remain usable; all repository verification passes.

**TDD/E2E cases:** profile picker filtering and invalid mapping states; disabled first-profile preview; enabled Codex-to-Claude route preview and telemetry; onboarding round-trip across restart; custom CTO identity unchanged after provider switch; mobile routing editor; no regression in direct Office task session selection or restart name preservation.

**Likely files:** Office routing types/store/hooks, routing page and provider-tier mapping, onboarding tier selectors, agent routing card/route strip, run detail attempts, the current dirty agent configuration handler/service/component changes, frontend unit tests, and Office Playwright specs.

**Verification:** `make -C apps/backend fmt`, backend test/lint, web typecheck/unit/lint, targeted desktop and mobile Playwright Office routing flows, and container-backed fallback E2E when supported.

**Dependencies:** Task 05.

**Output contract:** summarize user-visible changes, migration cleanup, all verification commands/results, files, screenshots/E2E evidence, and remaining limitations.
