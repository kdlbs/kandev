---
id: quick-chat-active-tab-visibility-01
title: Reveal active Quick Chat tab
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001
acceptance_criteria:
  - AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.1
  - AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.2
  - AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.3
  - AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.4
  - AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.5
system_design:
  - ../../specs/ui/system-design/quick-chat-active-tab-visibility.md
---

# Task 01: Reveal active Quick Chat tab

## Summary

After the separately authorized execution and HEAVY release, demonstrate a real hidden selection on unchanged production, then reveal newly active tabs with strip-local geometry and verify the user-visible result.

## Scope and exclusions

Own `apps/web/components/quick-chat/quick-chat-tab-strip.tsx`, a strip-local hook only if useful, `apps/web/components/quick-chat/quick-chat-tab-strip.test.tsx`, focused new desktop and mobile E2E files named in the plan, and a concise addition to the Quick Chat tabs paragraph in `docs/public/developer-tools.md`. Preserve all other guide content and concurrent changes. Existing quick-terminal and quick-chat-selection contracts retain selection, ordering, focus and lifecycle authority. No API, storage, session, process, global CSS, copy/catalog, observer, polling or speculative lifecycle changes.

## Implementation acceptance conditions

1. On unchanged production, a meaningful mounted geometry test fails for the missing reveal, and an isolated real overflowing browser row proves a hidden active identity through a reachable ordinary launch/setup/adjacent activation. Record actual geometry, selection, focus and vertical scroll before and after. Stop before production if focus already reveals it or no ordinary path reproduces; do not substitute a test-only store mutation for integration evidence.
2. Implement the design for both persisted wrapper kinds and temporary setup, only on initial selected mount or identity change. Prove minimum horizontal movement in both directions, no-op when visible, precise oversized-control behavior, unchanged selection/manual browsing, focus and vertical stability, and bounded reorder/drag and action preservation.
3. Focused desktop and configured phone flows use actual overflow, real keyboard/touch interaction, synthetic captures and no document horizontal overflow. Update only the relevant public guide paragraph and retain all other content. Promote spec and delivery statuses only after all linked evidence passes.

## ASCII UI preview

UI-01 (see [plan](plan.md#ascii-ui-preview)): selected item moves into the strip viewport; the selected content remains in place.

```text
Desktop: [ ... | Chat B | Terminal 3 actions ]
Phone:   [ ... | Terminal 3 ... ] [close]
         | selected terminal content          |
```

The row alone scrolls horizontally. Wide tabs reveal their label-bearing activation control; actions remain reachable by manual scrolling. This covers AC 001.1–001.5; compare actual desktop and phone captures against the structure.

## Dependencies, evidence and risks

Read the paired documents and scoped web guidance first. The selected row can be wider than the available viewport, sortable nodes use DnD refs and transforms, and setup uses a separate rendering path. Do not let effect dependencies turn title, activity, order or other rerenders into selection changes. Test scroll geometry in the mounted component and a real browser; mocked layout alone cannot prove the integration path. Use the existing E2E fixture and disposable data, not a developer instance. Preserve actual synthetic asset originals and manifests with hashes externally before another managed run. Before publication refresh actual main/source gap and fully paginated open-PR inventory for every actual owned path. The qualification recorded 137 open PRs and no strip/test/new-document overlap; guide overlaps were read as disjoint and must be rechecked, never picked up.

## Targeted verification commands

All commands below are **pending** the later release. Use pinned pnpm 9.15.9 first in PATH, then Node 24.21.0/bin and Go 1.26.0/bin; export `GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p2`. Install once from `apps` with `pnpm install --frozen-lockfile` before pnpm or hooks. Serialize work; reserve 8 GiB for web typecheck. Every `scripts/run-quiet` invocation or rerun must use a new task-owned external `KANDEV_RUN_QUIET_DIR`; retain and actually join every native session handle. The examples run from repository root. Set `Q` to a newly created unique task-owned external directory before **each** line; never reuse it. Use full logs for hooks; do not use `--summary` except E2E/playwright.

```bash
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet unit -- pnpm --dir apps/web test components/quick-chat/quick-chat-tab-strip.test.tsx
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet build -- make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package build-e2e-delivery-fixture
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet e2e --summary -- bash -c 'cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build --shards 1 --project chromium tests/chat/quick-chat-active-tab-visibility.spec.ts -- --workers=1 --retries=0'
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet e2e --summary -- bash -c 'cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-quick-chat-active-tab-visibility.spec.ts -- --workers=1 --retries=0'
```

First run the mounted test and relevant browser reproduction against unchanged production; record the expected RED and the ordinary hidden-selection evidence. Rebuild the owned web artifact after implementation and rerun the affected commands GREEN. Before running the second managed capture, save and hash the first run's assets and manifest externally. Do not infer success from a second run when the first handle has not joined. Then run these serially, with a fresh `Q` on each line:

```bash
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet unit -- pnpm --dir apps/web test components/quick-chat/quick-chat-tab-strip.test.tsx components/quick-chat/quick-chat-tab-item.test.tsx components/quick-chat/use-quick-chat-tab-order.test.ts
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet typecheck -- pnpm --dir apps/web run typecheck
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/quick-chat/quick-chat-tab-strip.tsx components/quick-chat/quick-chat-tab-strip.test.tsx e2e/tests/chat/quick-chat-active-tab-visibility.spec.ts e2e/tests/chat/mobile-quick-chat-active-tab-visibility.spec.ts
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet i18n -- pnpm --dir apps/web run i18n:check
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet i18n -- pnpm --dir apps/web run i18n:ratchet
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet docs -- node --test scripts/validate-public-docs.test.mjs
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet specs -- python3 scripts/list-docs.py validate
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet specs -- python3 scripts/lint-spec-files.test.py
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet specs -- python3 scripts/lint-spec-files.py --all
KANDEV_RUN_QUIET_DIR="$Q" scripts/run-quiet diff -- git diff --check
```

Include any actual new hook file in focused lint. Stage actual new frontend paths before the staged i18n ratchet. Run the same-worktree build targets above before `--no-build` E2E, with one worker and zero retries. Record actual exits, joins, geometry, asset hashes and limitations. No full suites.

## Results

The mounted test failed on the intended horizontal delta before implementation and passed after it. The ordinary launcher reopened an offscreen active terminal on unchanged production; after the change the same flow passed on desktop and phone. Focused mobile reorder coverage passed after the new test cleaned up its terminals. Web typecheck, focused lint, i18n, public docs and specification checks passed. The task plan records native handles, actual joins, logs and synthetic capture hashes.
