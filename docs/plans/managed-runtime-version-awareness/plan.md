---
spec: docs/specs/agents/requirements/runtime-updates.md
decision: docs/decisions/2026-08-12-validated-managed-runtime-version-selection.md
created: 2026-08-21
status: complete
---

# Implementation Plan: Managed Runtime Version Awareness

## Overview

Replace every unversioned managed npm ACP package invocation with an exact
effective version. The effective version is an install-wide validated operator
selection when present and a reviewed Kandev default otherwise. Build on the
existing version dialog and selection store, add a cached read-only update
status API and accessible blue-dot indicator, and add weekly automation that
opens reviewable pin-update PRs without changing runtimes automatically.

---

## Backend

### Central managed-runtime pins

- Add one machine-readable version catalogue at
  `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json` for
  Claude, Codex, OpenCode, Copilot, and Gemini. Package identity remains in the
  trusted agent metadata; the catalogue maps only those package names to exact
  stable SemVer defaults.
- Embed and validate the catalogue from a focused Go helper beside
  `managed_npm_runtime.go`. Startup must fail clearly if a trusted managed
  package has no valid default; no command may fall back to an unversioned
  package.
- Add `DefaultVersion` to `ManagedNPMRuntimeSpec`. `PackageSpec("")`,
  `CachedACPCommand()`, runtime configs, inference configs, and cache keys use
  the default. A non-empty validated override continues to produce
  `package@override`.
- Keep launch commands on `npx --yes --prefer-offline`. Candidate preparation
  and the existing bounded stale-metadata retry remain online-preferred.

### Effective version and operator selection

- Define one effective-version resolver: read
  `managed_runtime.active.<agent-id>` from the install-wide settings store,
  require the saved package to match trusted metadata, and otherwise return the
  built-in default.
- Route the effective version through every centrally built managed ACP command,
  including host utility probes, command previews, standalone sessions,
  containers, and SSH executors. Native-binary preference, passthrough commands,
  and separately distributed login helpers remain unchanged.
- Extend `managedruntime.SelectionStore` with deletion. The version job accepts
  a trusted `use_default` request, stages and ACP-probes the built-in default,
  then deletes the selection only after success. Candidate failure retains the
  prior selection and capabilities.
- Extend runtime catalogue, preview, and job DTOs with `default_version` and
  `effective_version`. Preserve `active_version` as the optional persisted
  operator selection. Add structural `use_default` operation copy; the browser
  does not infer operations from translated labels or version strings.

### Cached update status API

- Add `GET /api/v1/agent-update/status`. It returns one entry per available
  managed runtime with package, default, optional active selection, effective
  version, optional latest version, `checked_at`, and `check_state`.
- Implement a process-local, mutex-protected per-package cache in the agent
  settings controller. Successful npm `latest` results live for six hours;
  failed lookups produce `unknown` for fifteen minutes. Use injected clock and
  resolver seams in tests and bound concurrent stale lookups to the five trusted
  packages.
- Compare strict stable SemVer in the backend. Return `update_available` only
  when latest is newer than effective, `up_to_date` otherwise, and `unknown`
  when metadata is unavailable or invalid. One failed package does not fail the
  batch.
- Invalidate the affected cache entry after a successful version activation or
  return-to-default job. The dialog preview remains an uncached authoritative
  catalogue lookup.

---

## Frontend

### API and page-local status hook

- Add typed batch-status contracts to
  `apps/web/lib/api/domains/agent-update-api.ts` and the existing API barrels.
- Add a settings-domain hook that requests status when Settings > Agents mounts,
  after Rescan, and after a successful version job. Keep status page-local; the
  existing WebSocket-backed update-job store remains authoritative for active
  mutations.
- Treat `unknown` as no indicator, not as up to date. Do not disable the update
  trigger when the checker fails.

### Agent-card update indicator

- Pass the backend status for each agent through
  `apps/web/app/settings/agents/page.tsx` and
  `installed-agent-card.tsx` into the existing runtime update control.
- When `check_state` is `update_available`, overlay a blue dot on the existing
  refresh trigger. Add effective and latest versions to its accessible name and
  desktop tooltip; do not rely on color alone.
- Opening the trigger continues to fetch a live preview. The dot is a hint and
  never a separate action.
- Extend the existing dialog/drawer version selector with a clearly labelled
  Kandev-default option when an operator selection exists. The same state and
  mutation handler serve desktop and mobile.

### Mobile design contract

- **Desktop outcome:** the agent-group card shows a compact update hint and
  opens the existing centered version dialog.
- **Mobile entry point:** the same 44 px update trigger on the agent-group card.
- **Nearest exemplar:** the existing `AgentRuntimeUpdateControl`; retain its
  inset bottom `Drawer`, fixed header/footer, safe-area padding, and single
  scrolling body.
- **Hierarchy and action:** the dot signals availability; tapping the trigger
  opens the authoritative version summary and update/default action. No hover
  is required.
- **Geometry:** the dot does not change the trigger hitbox, add a scroll owner,
  or introduce horizontal overflow. Shared status and selection logic remain
  viewport-neutral.

### Localization

- Add copy for update availability, version interpolation, Kandev default, and
  unknown checker behavior to all five required locale catalogues.
- Generate Traditional Chinese catalogues through `pnpm run i18n:zh-hant` and
  run the focused i18n checks; do not hardcode labels in components.

---

## Weekly Pin Update Workflow

- Add `scripts/update-agent-runtime-pins.mjs` to read the central catalogue,
  query each trusted package's npm `dist-tags.latest`, reject prerelease or
  invalid SemVer values, and rewrite only changed pins. Keep parsing and rewrite
  logic independently testable with fixture metadata and no network.
- Add `.github/workflows/update-agent-runtime-pins.yml` with weekly and manual
  triggers, least-privilege permissions, pinned action SHAs, a stable bot
  branch, and one grouped Conventional Commit PR. Use a repository-approved
  bot/App token so the generated PR receives normal CI events. Do not auto-merge.
- Run the updater tests, targeted managed-runtime Go tests, action-pinning lint,
  and workflow contract test before opening a PR. When no pin changes, exit
  without a branch or PR mutation.
- Add a workflow contract test under `.github/scripts/` and wire it into
  `lint-action-pinning.yml` so schedule, permissions, token boundary, validation,
  and no-auto-merge behavior cannot silently drift.

---

## Tests

- **Exact defaults and overrides:** table-driven argv tests cover all five
  packages, empty-selection fallback, explicit selection, native preference,
  host utility, standalone, container, and SSH command construction.
- **Selection lifecycle:** real settings-store tests cover save, matching package,
  default fallback, delete-on-success, and preservation on failed candidate
  validation.
- **Update status:** controller and handler tests cover newer/equal/older latest,
  partial registry failure, strict SemVer, cache TTLs, invalidation, and one
  read-only batch response without starting a job.
- **Frontend status:** API and hook tests cover successful, partial-unknown, and
  post-update refresh behavior. Component tests cover dot visibility,
  accessible version copy, usable unknown state, and return-to-default action.
- **Workflow:** offline updater fixtures and workflow-contract tests cover
  changed/no-change/prerelease/error cases and secure PR creation.

## E2E Tests

- **Desktop update hint:** given a newer registry version, opening Settings >
  Agents shows the blue dot; opening it shows the authoritative current-to-latest
  preview and starts no mutation until approval.
- **Unknown status:** given one failed package lookup, that agent has no dot but
  its update dialog remains usable; another package can still show an update.
- **Post-update state:** after a successful update, the status refresh removes
  the dot and the selected version survives page reload.
- **Mobile parity:** on Pixel 5, the dotted 44 px trigger opens the existing
  drawer, exposes the version information without hover, keeps the action
  reachable, and produces no document horizontal overflow.

Files:

- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`

---

## Public Documentation

- Update `docs/public/agents-and-profiles.md` with shipped defaults, explicit
  selections, the blue update hint, return-to-default behavior, and the accepted
  npm/network/cache limitations.
- Update `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md` so every
  documented command uses `package@effective-version` and distinguishes normal
  offline-preferred launches from online-preferred updates.
- Search public and internal docs for claims that managed packages are
  unversioned or host-only and reconcile only affected sources.

---

## Verification Results

Pre-remediation baseline. Backend focused tests passed 2,614 tests across seven
packages and `make -C apps/backend lint` reported zero issues. Frontend focused
tests passed 22/22, full web lint and typecheck passed, and the i18n gate
passed. Desktop and mobile E2E passed 14/14 and 4/4. Updater,
workflow-contract, action-pinning, and `zizmor` checks passed. Public-doc tests
passed 61/61 and the validator accepted 41 pages. `git diff --check` passed.

### Review remediation

The review pass corrected existing-PR selection and App-token propagation in
the weekly workflow, parsed GitHub output flags before catalogue selection,
preserved structural `use_default` intent, omitted failed lookup timestamps,
and retried failed page-local status refreshes. The second remediation pass
also applied least-privilege workflow permissions, package-safe selection
projection, bounded status lookups, shared default derivation, nil-cache
initialization, and default restoration when the active version is unknown.
The follow-up review pass updates terminal `current_version` from the
successful capability probe, makes the UI prefer terminal `effective_version`
and retain the complete backend version projection, derives managed-runtime
test expectations from the catalogue, and validates those commands before the
weekly workflow commits or pushes. It also keeps the App token on the push
step and refreshes existing updater PRs by number.

Follow-up verification passes 2,594 focused backend tests, 32 focused web
tests, full web lint/typecheck/i18n, 7 pin updater tests, 8 workflow-contract
tests, 9 action-pinning tests, the 19-workflow action linter, `zizmor`, and
desktop/mobile E2E at 15/15 and 4/4. The aggregate backend phase of `make
test` passed; its full web Vitest phase did not reach a terminal summary, so it
was interrupted after the isolated `http-git-server` suite passed 3/3.

---

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation unless the user explicitly
authorizes subagents after choosing the implementation model.

Wave 1:

- [x] [Task 01: Add reviewed default pins](task-01-default-pins.md)

Wave 2:

- [x] [Task 02: Resolve and persist effective versions](task-02-effective-version-selection.md)
- [x] [Task 04: Automate weekly pin PRs](task-04-weekly-pin-workflow.md)

Task 04 is parallel-safe after Task 01 because it owns workflow and script files;
Task 02 owns runtime selection and API mutation files.

Wave 3:

- [x] [Task 03: Add cached update status](task-03-update-status-api.md)

Wave 4:

- [x] [Task 05: Show update awareness in Settings](task-05-settings-update-indicator.md)

Wave 5:

- [x] [Task 06: Prove desktop and mobile flows](task-06-update-awareness-e2e.md)
- [x] [Task 07: Update runtime documentation](task-07-runtime-version-docs.md)

Task 07 is parallel-safe after Tasks 01-05 because it owns documentation only;
Task 06 owns E2E fixtures and specs.

---

## Risks

- An exact top-level npm package does not lock transitive ranges. The design
  intentionally accepts npm/cache/network behavior rather than claiming full
  artifact reproducibility.
- A user selection is install-wide and affects remote commands built by the
  same backend, but candidate ACP validation runs on the Kandev host. A remote
  platform can still reject a host-validated package and must surface its normal
  launch error.
- Registry checks must not delay application boot or agent launches. They are
  page-triggered, read-only, cached, and separate from the agent catalogue boot
  payload.
- A bot-created PR must trigger the repository's normal required checks. The
  workflow must not silently fall back to `GITHUB_TOKEN` behavior that suppresses
  those PR events.
