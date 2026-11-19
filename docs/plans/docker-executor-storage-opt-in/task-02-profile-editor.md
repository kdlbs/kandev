---
id: "02-profile-editor"
title: Profile editor and public reference
status: done
wave: 2
depends_on:
  - "01-profile-and-launch"
plan: plan.md
requirements:
  - REQ-EXECUTORS-STORAGE-001
acceptance_criteria:
  - AC-EXECUTORS-STORAGE-001.4
  - AC-EXECUTORS-STORAGE-001.5
  - AC-EXECUTORS-STORAGE-001.6
  - AC-EXECUTORS-STORAGE-001.8
system_design:
  - ../../specs/executors/system-design/docker-storage-bounds.md
---

# Task 02 Profile editor and public reference

## Summary

Add one Storage limits card to the Docker profile editor. Let authorized users
save or clear limits from desktop and phone through the current settings shell.

## In scope

- Add the inputs after Container networks in `DockerSections`. Include them
  in the existing draft, discard, and save logic. Show example values.
- Use `buildSaveConfig` to preserve unrelated keys and omit cleared settings.
  Localize the new copy in the executor catalogs. Generate Traditional Chinese
  with `i18n:zh-hant`.
- Update the Docker log and temporary storage limits section of
  `docs/public/executors.md`. Describe the saved values and blank defaults.
  Document unsupported drivers, the `json-file` count caveat, mount
  precedence, tmpfs memory use, and the fresh-container boundary.

## Out of scope

Connection setup and daemon settings have separate editor flows. This task
uses the canonical executor profile editor that both Docker creation flows open.

## Acceptance

1. Both Docker profiles can set, save, reload, clear, and discard the limits.
   Blank fields stay blank until edited. Unrelated profile keys survive saves.
2. Phone controls fit the viewport and support touch save and discard.
3. The public reference describes the new defaults and gives example values
   for each optional limit. It explains when Docker can apply rotation.

## ASCII UI preview

This work order owns UI-01 and UI-02. See the [full plan preview](plan.md#ascii-ui-preview).

**UI-01 Desktop**

```text
Storage limits
Temporary storage (/tmp) [       ]    Example 2g
Maximum log file size    [       ]    Example 100m
Maximum log files        [       ]    Example 3
                         [Reset]   [Save changes]
```

**UI-02 Phone**

```text
Storage limits
Temporary storage (/tmp)
[                          ]
Maximum log file size
[                          ]
Maximum log files
[                          ]
[Reset]    [Save changes]
```

The desktop settings card gives the three fields a consistent label column.
The phone card puts labels above full-width fields in the same scroll surface
as the profile. The existing floating save control handles viewport and safe
area. Follow the touch input sizing from the neighboring Container networks
card. Compare both rendered views with the preview during the browser checks.

## Verification

From the repository root, run the workspace installation first in a fresh
worktree. Then run the frontend checks, each rooted at `apps/web`:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/settings/profile-edit/serialize-executor-config.test.ts components/settings/profile-edit/executor-profile-reset.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/settings/docker-storage-limits.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/settings/mobile-docker-storage-limits.spec.ts)
```

Run the two Playwright commands in sequence. From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/components/settings/profile-edit/{docker-storage-limits-card.tsx,profile-runtime-sections.tsx,serialize-executor-config.ts,serialize-executor-config.test.ts,executor-profile-reset.test.tsx}`
- `apps/web/src/locales/<locale>/executors.json`
- `apps/web/e2e/tests/settings/{docker-storage-limits.spec.ts,mobile-docker-storage-limits.spec.ts}`
- `docs/public/executors.md`

## Dependencies

Task 01 supplies the profile settings and container behavior.

## Risks

Save and discard share the rest of the profile form. Check that clearing a
storage limit preserves unrelated profile values.

## Parallelism

Sequential.

## Inputs

[Requirements](../../specs/executors/requirements/docker-storage-bounds.md) · [System design](../../specs/executors/system-design/docker-storage-bounds.md) · [Mobile Docker settings example](../../../apps/web/e2e/tests/settings/mobile-docker-networks.spec.ts)

## Results

Complete. Both Docker profile types now expose the optional limits through the
existing draft, reset, and save flow. Saves preserve unrelated profile keys,
and blank fields clear their settings. Untouched fields follow refreshed profile
data; edited fields, including explicit clears, remain in the save payload.
The temporary-storage label states that the `/tmp` mount is in memory in all
locales. All 26 focused profile tests passed. The web typecheck and lint, i18n
checks and ratchet passed. Desktop and mobile E2E tests passed for Local Docker
and Remote Docker profiles. Public-doc tests passed (63 tests), and all 55
published docs pages validated. Task 01 results are recorded in the parent
plan. The whitespace-save refresh regression and post-clear reload assertion
passed. Specification gates `python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all` passed. The diff gate
`git diff --check` passed.
