# SSH access experience: local verification

Implementation authorized by the user's “go for it” after the four-work-order
package. This record belongs to task `a2845dea-8086-4b3a-824c-65e1941d4cd8`,
session `13973a35-0812-478d-a620-2e36314c61f1`, branch
`feature/recover-and-resume-s-1mt`, based on local merge `1967906b5b0b`.
No commit, push, GitHub message, deployment, or live SSH authorization occurred.

## Implemented

- Access milestones, inline server trust review, full labelled fingerprints,
  operation-specific feedback, and a bastion setup path.
- Optional installation replaces the Access body. Users explicitly choose
  password, existing identity, or manual installation. Review precedes the only
  remote mutation. Passwords clear on dispatch and decision invalidation.
- Distinct pending, verified, failed, installed-unverified, and unknown results.
  Leaving a pending form does not claim to cancel remote work.
- Diagnostics report actual local setup issues by hop, offer existing remedy
  destinations, and keep live SSH testing separate. Identity remedies return to
  the source section using allowlisted `return_section` context.
- A feature- and administrator-gated SSH connections leaf before executor
  records. The shared settings tree serves desktop and phone; profile counts
  and flat mode retain their meanings.
- All supported locales and public executor instructions updated.

## TDD and checks

Focused RED receipts under `/tmp/ssh-experience-*`: operation context (2), inline
trust invalidation (3), installation experience (4), adapted installation
contracts (10), diagnostic report (3), sidebar eligibility (2), remedy routing
(2), diagnostic return (1), and trust-revision focus (1). Final focused unit
run passed 169 tests in 19 files. After the last markup-only selection polish,
the complete SSH component directory passed 78 tests in 13 files.

Scoped Oxlint, web typecheck, i18n key completeness and new-copy ratchet passed.
Public documentation validation passed 55 pages and 63 validator tests.
Specification catalog/lint and 36 linter tests passed. Local `validateCoverage`
accepted all four work orders and their linked contracts without GitHub access.

Browser commands use the guarded runner, `--host`, one project at a time and
fresh e2e web assets. On this Nix host `/tmp/ssh-experience-run.py` supplies the
installed Chromium shared libraries to the child process and removes inherited
`KANDEV_*` environment variables. It changes no shell or service configuration.

- Desktop: 23 checks passed across access setup, diagnostics, navigation,
  profile continuation, identity setup and management layout. Final polish
  capture rerun: 3 checks passed.
- Phone: 16 combined checks passed, including coarse tablet navigation.
- Auth permissions: 1 check passed. A new assertion initially matched the
  breadcrumb as well as navigation; it was scoped to the sidebar and rerun.
- Real SSH: all 12 selected cases passed across the initial run and targeted
  rerun. This includes direct/bastion password enrollment, bastion identity
  enrollment, preservation of existing authorization entries, authentication
  after restart, reusable executor launch, and desktop/phone adoption.

Real SSH used `/tmp/kandev-ssh-verify-20261115/docker/docker.sock`. The initial
run passed 10 cases; two bastion fixtures failed while installing the keyscan
helper because their temporary network lacked outbound routing. A first retry
found orphan fixture port bindings. The fixture-only network rules were fixed,
orphan containers were removed from this daemon, and both bastion cases then
passed. No product change or weakened host-key check was used to fix the fixture.
The daemon, containers, bridges, and all labelled verification firewall rules
were removed after the tests. The manual lab and main `:9998` instance were not
changed. Retained image layers and logs are under the temporary verification root.

## Reproduction

From the repository root, with browser libraries available:

```bash
pnpm --dir apps/web exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx lib/api/domains/ssh-connections-api.test.ts components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx lib/settings/ssh-connection-settings-routes.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-diagnostics.spec.ts tests/settings/ssh-navigation.spec.ts tests/settings/ssh-profile-connection-link.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-diagnostics.spec.ts tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-profile-connection-link.spec.ts tests/settings/mobile-ssh-connections.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --project auth tests/auth/ssh-connections-permissions.spec.ts
```

For real SSH, provision a separate disposable daemon and export its `DOCKER_HOST`
and Docker CLI path. This checkout has no reachable release tag, so build with a
valid fixture version (`VERSION=0.100.0`); the compact-runtime fixture rejects a
raw dirty git SHA as its release version. Then run the containers project with
`KANDEV_E2E_CONTAINERS=1` and `tests/ssh/{managed-enrollment,managed-connections,managed-adoption-browser}.spec.ts`.
Never use the manual lab's daemon for these tests.

## Visual evidence and limits

Inspected desktop and phone method choice, inline review, pending/unknown
results, host trust, and diagnostic issue/failure states in light and dark.
Pseudo-locale method choices and narrow/coarse layouts passed containment
checks. Existing verified-badge checks measured at least 4.5:1 contrast in both
themes. Control geometry checks confirmed 28px desktop and at least 44px touch
actions. Final captures are in the ignored workspace directory
`apps/web/e2e-artifacts/ssh-access-experience-20261115/{desktop,phone}/`.
The shared floating assistant control remains part of the existing shell.
Earlier screenshots are
under `/tmp/ssh-experience-evidence/`; they are UI transport fixtures and do not
constitute real SSH evidence. Real SSH coverage is recorded separately above.
Changes remain local and uncommitted. The manual test instance still serves its
previous build; refreshing it is outside this package's implementation scope.
