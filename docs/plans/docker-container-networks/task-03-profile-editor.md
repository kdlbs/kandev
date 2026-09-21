---
id: "03-profile-editor"
title: "Executor profile network editor"
status: done
wave: 3
depends_on: ["01-primary-network", "02-additional-networks"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-003
acceptance_criteria:
  - AC-EXECUTORS-DOCKER-NETWORKS-003.1
  - AC-EXECUTORS-DOCKER-NETWORKS-003.2
  - AC-EXECUTORS-DOCKER-NETWORKS-003.3
  - AC-EXECUTORS-DOCKER-NETWORKS-003.4
system_design:
  - ../../specs/executors/system-design/docker-container-networks.md
---

# Task 03: Executor Profile Network Editor

## Summary

An operator sets a Docker profile's primary network, its gateway priority, and
its additional network attachments from the executor profile editor, and can
tell from the empty state which default applies to that profile's daemon.

## In scope

- Add a `DockerNetworkCard` to
  `apps/web/components/settings/profile-edit/docker-sections.tsx`, rendered from
  `profile-runtime-sections.tsx` for `isDocker` profiles only, between the
  Dockerfile build card and the user-namespaces card.
- Add a form-state hook beside `use-user-namespaces-form-state.ts` owning the
  primary name, the primary gateway priority, and the additional list, with a
  baseline reset, and wire it into both the edit and the create profile pages.
- Write `docker_network`, `docker_network_gw_priority`, and
  `docker_additional_networks` in `buildSaveConfig`, gated on `form.isDocker`,
  so switching to a non-Docker executor type clears them and an untouched
  profile persists no network configuration.
- State in the empty-state helper text that the daemon's own default network
  applies. It reads the same for both Docker executor types.
- Localize all copy through `t()` in the five shipped locales, using
  `pnpm run i18n:zh-hant` for the Traditional Chinese pair.

## Out of scope

- Any backend behavior (tasks 01 and 02).
- Validating a network against a daemon from the editor. The editor validates
  name syntax; existence and driver are a launch-time check.
- A network picker populated from the daemon, or any network create or delete
  control.
- Playwright coverage (task 04).

## Acceptance

- The network card appears for `local_docker` and `remote_docker` profiles and
  for no other executor type, with the primary field, its gateway priority, and
  an add/remove additional-network list matching the plan's UI-01 and UI-02
  previews.
- The empty-state helper text states that the daemon's own default applies.
- Saving an untouched profile persists no network keys; saving a configured one
  round-trips every value.
- The phone composition stacks each additional network as its own card with a
  labelled priority field and an in-card remove control.

## Verification

```sh
cd apps/web && pnpm run test -- serialize-executor-config use-docker-networks-form-state
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

New tests must cover: the serializer writing and clearing each key by executor
type; the form-state hook's baseline reset, add, remove, and priority edit; and
that an empty form produces no network keys.

## ASCII UI preview

This work order implements plan previews `UI-01` and `UI-02`. See
[the plan's ASCII UI previews](plan.md#ascii-ui-previews) for the full
drawings. Compare the rendered card against the structural requirements stated
there during verification, and record any difference rather than drifting from
it silently.

## Likely files

- `apps/web/components/settings/profile-edit/docker-sections.tsx`
- `apps/web/components/settings/profile-edit/profile-runtime-sections.tsx`
- `apps/web/components/settings/profile-edit/serialize-executor-config.ts`
- `apps/web/components/settings/profile-edit/use-docker-networks-form-state.ts` (new)
- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/app/settings/executors/new/[type]/page.tsx`
- `apps/web/src/locales/*/`

## Dependencies and risks

Depends on tasks 01 and 02 for the key names and their value shapes; a mismatch
here produces a profile that saves and silently does nothing.

Run `/mobile-parity`: an additional-network row is a horizontal composition that
does not survive a narrow viewport, and shrinking it is not a phone design. The
helper text is the only place the install-wide default and the remote-daemon
exception are explained to an operator, so it is a requirement, not decoration.
