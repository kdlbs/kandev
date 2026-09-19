---
spec: docs/specs/agents/requirements/portable-agent-configuration.md
decision: docs/decisions/2026-08-15-portable-agent-configuration-bundles.md
created: 2026-08-15
status: implemented
---

# Implementation Plan: Portable Agent Configuration

## Overview

Add an optional executor-profile control for small agent configuration files.
Keep authentication and configuration as separate choices.

The backend will own the file allowlist and file transfer.
The frontend will store selected bundle IDs in the existing profile configuration map.

## Architecture

### Provider declarations

- Add an optional portable-configuration capability beside `RemoteAuth`.
- Add one catalog package for stable bundle IDs and host-file availability.
- Declare initial bundles for Claude, Codex, and OpenCode.
- Remove `config.toml` from the Codex authentication file list.
- Keep agents without a safe declaration out of the catalog.

### API and state

- Add `GET /api/v1/agent-config-bundles` to the executor-profile handler group.
- Return bundle metadata, source paths, target paths, and host availability.
- Do not return file data through the API.
- Store selected IDs in `ExecutorProfile.Config.agent_config_bundles` as JSON.
- Pass this profile key into lifecycle launch metadata.
- Keep the raw file data out of the database and frontend state.

### Transfer engine

- Resolve selected IDs against the backend catalog during fresh provisioning.
- Read only regular files below the Kandev host home.
- Reject symbolic links, absolute targets, path traversal, and size-limit violations.
- Limit one file to 1 MiB and one launch to 4 MiB.
- Write target files with mode `0600`.
- Preserve the declared relative target path.
- Report a preparation warning and continue after an optional copy error.

### Executor integration

- Local Docker writes selected files into its Kandev-managed agent session directory.
- SSH writes selected files under the configured remote user home.
- Sprites writes selected files under the sandbox home.
- Fresh provisioning and environment reset copy current host data.
- Warm resume keeps the executor copy and does not read the host again.
- Local and Worktree do not use this transfer path.

### Frontend

- Add the configuration controls to each matching agent row in the existing remote-credentials settings card.
- Keep each configuration checkbox independent from that agent's authentication radio group.
- Do not render a separate global agent-configuration section.
- Use the existing route-level save coordinator and dirty-state markers.
- Show unavailable host files without removing a saved selection.
- Add translated copy for all labels, descriptions, warnings, and states.

## Desktop and mobile behavior

The desktop and mobile entry point is **Settings > Executors > profile**.
The nearest shipped exemplar is the warning help in `sleep-inhibition-settings.tsx`.

The existing settings page remains the only vertical scroll owner.
The configuration controls stay inside the existing remote-credentials card.

The warning icon uses a tooltip for a fine pointer.
It uses a bottom drawer for a coarse pointer.

The warning control has a 44 by 44 CSS pixel touch target.
Each checkbox row also has a minimum 44 CSS pixel height.

The visible description gives the main risk without the tooltip.
The tooltip or drawer gives the full list of effects.

## Warning content

The warning must tell the user these facts:

- Kandev copies the selected file without changes.
- The file can contain secrets or environment values.
- The file can add hooks or commands in the executor.
- The file can change models, permissions, MCP servers, and endpoints.
- The file can contain host paths that do not work in the executor.
- Fresh provisioning can replace an existing target file.
- An SSH copy can affect other processes that use the same remote account.

## Test strategy

### Backend unit and integration tests

- Build catalogs for Linux, macOS, missing files, and agents without bundles.
- Make sure that stable IDs do not depend on declaration order.
- Make sure that Codex authentication contains `auth.json` only.
- Cover regular files, symbolic links, traversal, missing files, modes, and size limits.
- Cover fresh provisioning, warm resume, and environment reset.
- Cover Local Docker, SSH, and Sprites transfer adapters.
- Cover unknown saved IDs and best-effort warnings.

### Frontend tests

- Keep configuration and authentication selections independent.
- Keep dirty state and save serialization correct.
- Keep a missing saved selection visible and removable.
- Cover tooltip focus and hover behavior.
- Cover coarse-pointer drawer behavior.
- Cover translated labels and accessible names.

### E2E tests

- Add a desktop settings test for selection, save, reload, and independent authentication.
- Add a mobile test for tap behavior, drawer content, touch targets, and horizontal overflow.
- Add a container test that proves the selected file appears in a fresh Docker agent home.
- Prove that an unselected file does not appear.
- Prove that warm resume does not overwrite the executor file.

## Public documentation

Update `docs/public/executors.md` as a how-to and security explanation.
Document selection, fresh-copy timing, resume behavior, file limits, and SSH overwrite risk.

Update `docs/public/security.md` with the new host-file authority grant.
State that configuration can contain secrets and executable hooks.

## Required companion plan

Configuration copying does not guarantee equal host and executor model catalogs.
The [executor model mismatch plan](../executor-model-mismatch-fallback/plan.md) handles the remaining difference.

That plan keeps task launch operational and persists a warning in task chat.
It works when configuration copying is off or cannot create parity.

## Implementation waves

Wave 1:

- [x] [Task 01: Define portable configuration catalog](task-01-portable-config-catalog.md)

Wave 2:

- [x] [Task 02: Transfer selected configuration bundles](task-02-configuration-transfer.md)
- [x] [Task 03: Add executor configuration controls](task-03-executor-config-controls.md)

Tasks 02 and 03 are parallel-safe after Task 01.
They have separate backend-runtime and frontend file ownership.

Wave 3:

- [x] [Task 04: Prove executor configuration behavior](task-04-executor-config-evidence.md)

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/agents ./internal/agent/remoteconfig ./internal/task/handlers
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor
cd apps && pnpm --filter @kandev/web test -- --run components/settings/profile-edit lib/api/domains
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/settings/executor-agent-config.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-executor-agent-config.spec.ts
cd apps/web && pnpm e2e:run --project containers tests/docker/agent-config-copy.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Risks

- A copied file can run provider hooks with the authority of the agent process.
- An SSH copy can replace shared remote-user configuration.
- Provider file formats can change outside Kandev releases.
- A host path inside the file can be invalid in the executor.
- Removing implicit Codex configuration copy can change existing Docker behavior.
- Best-effort transfer can leave older configuration in a reused remote home.

## Results

- Implemented the catalog, safe transfer engine, Local Docker/SSH/Sprites wiring,
  independent settings controls, and translated desktop/mobile guidance.
- Backend targeted tests: 7,299 passed across 45 packages.
- Frontend typecheck, lint, i18n checks, focused component tests, and E2E sleep
  ratchet passed.
- Desktop, mobile, and gated Docker E2E scenarios passed. Public-doc validation
  passed with 61 tests and 41 published pages.

## Out of scope

- The provider-neutral model behavior in the required companion plan.
- Arbitrary file selection.
- File content preview or edit functions.
- Project configuration that already travels with the repository.
- Runtime-state databases, sessions, caches, and lock files.
