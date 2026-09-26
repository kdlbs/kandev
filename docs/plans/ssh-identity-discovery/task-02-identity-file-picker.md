---
id: "02-identity-file-picker"
title: "Identity file picker with custom path"
status: done
wave: 2
depends_on:
  - "01-identity-discovery-endpoint"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-IDENTITY-DISCOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.4
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.5
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.6
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.7
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.8
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.9
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.10
system_design:
  - ../../specs/executors/system-design/ssh-identity-discovery.md
---

# Task 02: Identity File Picker With Custom Path

## Summary

Replace the free-text identity file input in `SSHConnectionForm` with a
combobox over discovered keys plus a terminal custom-path option, falling back
to the current text input whenever discovery is unavailable. The change is
inherited by both the SSH and Remote Docker connection cards.

## Scope

- `listSSHIdentities()` in `apps/web/lib/api/domains/ssh-api.ts`, via
  `fetchJson` so it honors `apiBaseUrl`.
- `apps/web/components/settings/ssh-identity-file-field.tsx` (new), built on
  `apps/web/components/combobox.tsx`, owning: fetch on first render with
  `identity_source === "file"`, option ordering per system design D4, the
  `__custom__` sentinel, custom mode, and loading / empty / error / unavailable
  states.
- Swap the field in `ssh-connection-form.tsx`. `onChange("identity_file", …)`
  keeps its current signature so the card's `CONNECTION_FIELDS` staleness
  handling applies unchanged.
- New copy in `en` and `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `pseudo`. Use
  `pnpm run i18n:zh-hant` for the Traditional pair.

## Exclusions

- No change to the persisted config shape: `ssh_identity_file` stays a string.
- No change to `ssh-connection-card.tsx` test/trust/save logic.
- No new UI primitive; extend nothing in `@kandev/ui`.
- No e2e spec in this work order.

## ASCII UI preview

`UI-01: Identity file control`, as drawn in
[`plan.md`](plan.md#ui-01-identity-file-control-identity-source--file).
Collapsed, expanded, custom-path, unavailable, and empty states are specified
there; the unavailable state must render byte-identically to today's field.

## Implementation acceptance conditions

1. With discovery returning one selectable and one encrypted key, the combobox
   lists both, the encrypted one is not selectable and states that ssh-agent is
   the route for it, and choosing the selectable one calls
   `onChange("identity_file", "<its path>")`.
2. A saved `identity_file` absent from the discovery result renders in custom
   mode with the value intact; a 403 and a rejected request each render the
   plain text input with that value intact.
3. `__custom__` is a `===`-compared sentinel that appears in no locale file,
   and `pnpm run i18n:check` passes with the new copy present in all five
   languages.

## Verification commands

```
cd apps/web && pnpm vitest run components/settings/ssh-identity-file-field.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
```

## Likely files

- `apps/web/components/settings/ssh-identity-file-field.tsx` (new)
- `apps/web/components/settings/ssh-identity-file-field.test.tsx` (new)
- `apps/web/components/settings/ssh-connection-form.tsx`
- `apps/web/lib/api/domains/ssh-api.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/executors.json`

## Dependencies and risks

- Depends on task-01's response shape.
- `/mobile-parity` applies: the option list must be touch-usable, not a
  narrowed desktop dropdown. Verify the phone composition described in UI-01.
- The existing `sshIdentityFileHint` copy is replaced, not duplicated; leaving
  both would state the passphrase rule twice in different words.

## Results

Implemented. `ssh-identity-file-field.tsx` renders the discovered keys through
the shared combobox with a custom-path row; `FieldShell` moved to
`ssh-field-shell.tsx` so the new field and the form can share it without a
circular import. The replaced `sshIdentityFileHint` key was removed from all
six catalogs rather than left orphaned.

- `pnpm vitest run components/settings/ssh-identity-file-field.test.tsx` — 7 passed
- `pnpm vitest run components/settings/` — 196 files, 1267 tests passed (no regression from the shell extraction)
- `pnpm run typecheck` — clean
- `pnpm run i18n:check` — all six checks pass; five languages complete, pseudo in sync
- `pnpm --filter @kandev/web lint` — 0 warnings
