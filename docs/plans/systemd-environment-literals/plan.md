---
created: 2026-10-17
status: implemented
requirements:
  - REQ-CLI-NATIVE-KANDEV-CLI-001
system_design:
  - ../../specs/cli/system-design/native-service-configuration.md
legacy_specs: []
---

# Implementation Plan: Systemd Environment Literals

## Overview

Preserve literal dollars in generated Linux service environments while retaining
the existing command and PATH-specifier contracts. Deliver one sequential work
order: independently author renderer regressions, observe the causal failure,
correct the environment quoting context, and run the bounded launcher checks.

The owning requirement is
[Native Kandev CLI](../../specs/cli/requirements/native-kandev-cli.md), specifically
AC-CLI-NATIVE-KANDEV-CLI-001.9 through 001.11. This package does not promote or
migrate the surrounding draft CLI material. The historical
[service identity package](../system-service-identity-guardrails/plan.md) is
complete and unaffected; its broad legacy verification commands do not apply.

## Confirmed cause and existing evidence

`renderSystemdUnit` calls `serviceEnvLine` and
`serviceEnvLineAllowSpecifiers`, which reuse command quoting that doubles `$`.
Systemd `Environment=` has no dollar expansion, so that duplication reaches the
process unchanged. The two existing helper tests explicitly expect the wrong
environment strings and must be corrected against the real grammar.

ROOT qualified clean main `805f90e7beb813e7d44f0902bb77540c14fcd193` using the
actual renderer's exact `Environment` property and a real user-systemd transient
Python process that read only fixture `KANDEV_HOME_DIR` and checked its directory:

- Causal failure: `home$literal` rendered and arrived as `home$$literal`; the
  received directory did not exist.
- Passing controls: a rendered plain home existed; the same manager received a
  literal-dollar value unchanged when passed directly, and that directory existed.
- All three unique disposable units were collected (`LoadState=not-found`). No
  existing Kandev service, config, or manager setting was changed. No backend,
  agent, or database was launched. Installed Kandev startup and database damage
  were not tested.

Receipt-only evidence is under
`/tmp/kandev-root-systemd-env-discovery-0l2orsu5/`: `qualified-proof.json`,
`qualification-receipt.json`, and `qualification.log`. Native run 82576 joined
as `60d5d2`: wrapper exit 0, expected Go regression exit 1, own group 274849
absent, temporary in-repo source checksum-proved removed. Private source is
read-only with SHA-256
`df3d6e04859a1c903dfa3db493f73bc350a0c9f472555ab0f9815f16588da619`.
Never read, copy, replay, or delete `candidate_test.go` or `run.py`. Permanent
regressions are authored independently from repository patterns and documented
semantics. Do not replay unchanged ROOT qualification.

Design inspection confirmed matching HEAD/main and implicated blobs:
`service.go` = `2d4a9b74d22732ff9fc7f12cb30177b1b976540a`;
`service_test.go` = `b0a5076468106788dec462eab3ddd8d48c4b2714`.
Recheck if main changes before implementation; do not rebase for drift alone.

## Scope and technical approach

- In `apps/backend/internal/launcher/service.go`, give environment quoting a
  distinct dollar policy; preserve command quoting, ordinary character escaping,
  literal percent escaping, and trusted PATH specifiers.
- Correct the two erroneous assertions in `service_test.go`; add focused
  full-renderer regressions and controls in `service_environment_test.go`.
- Keep the requirement/design/manifest/work-order record synchronized without
  promoting unrelated draft requirement content.

Excluded: launchd production changes, schema/API/SDK/flags/defaults, identity and
ownership policy, updater or lifecycle work, service-manager rewrites, shell
injection policy, templates/scripts, dev launcher, passthrough, stopped UTF-8
work, installation/restart of real Kandev services, and persistent units.

## Compatibility

| Surface | Intended behavior | Evidence |
| --- | --- | --- |
| Linux user and system Environment | Exact literal dollars and ordinary values | Full-renderer tests in both scopes; ROOT external user-manager qualification |
| Ordinary environment percent | Literal percent encoded as `%%` | Exact assignment expectations |
| Trusted base PATH `%h` | Specifier preserved; percent prefixes still rejected | Renderer and existing prefix-filter controls |
| Linux ExecStart | Existing command escaping and argv retained | Exact full-renderer command-line control |
| macOS launchd | Existing XML and argv delivery retained | Full-plist control; no native macOS runtime claim |

## Tests and external boundary evidence

`service_environment_test.go` will own:

- `TestRenderSystemdUnitPreservesEnvironmentLiterals`: both scopes and all emitted
  path settings, including selected config with configuration-owned home omitted;
  AC 001.9 and 001.10. The single-dollar fixture must fail before production
  changes for dollar duplication, while plain-path controls pass.
- `TestRenderSystemdUnitPreservesPathLiteralsAndSpecifiers`: accepted dollar
  prefix plus trusted `%h`, and rejected percent-bearing prefix; AC 001.9 and
  001.11.
- `TestRenderSystemdUnitKeepsCommandQuoting`: dollar/percent/space/quote/backslash
  executable control with `--headless` unchanged.
- `TestRenderLaunchdPlistPreservesEnvironmentLiterals`: unchanged literal dollar,
  percent and XML escaping in environment and program arguments.

In `service_test.go`, rename the misleading
`TestQuoteForUnitEscapesSystemdSpecials` to describe its environment contract and
correct its dollar expectation. Correct
`TestServiceEnvLineAllowSpecifiersPreservesSystemdSpecifiers` while keeping its
trusted-specifier assertion. Neither correction replaces the renderer tests.

ROOT's manager qualification provides external process-boundary evidence;
permanent tests stay portable and assert complete expected output lines without
a fabricated parser. No browser E2E is applicable. No additional manager smoke
is scheduled; one would need separate justification and authorization with
unique disposable units, never replay of ROOT's fixture.

## Documentation judgment

Reviewed CLI README and public CLI/service guidance through `/docs-maintainer`.
This package records internal repair intent. No operator command, installation
step, configuration key, or ownership policy changes. No public workaround or
installation rewrite is useful for this correction; keep public docs unchanged.

## Work orders

- [x] [Task 01: Quote service environment values](task-01-quote-service-environment.md)

No dependencies or parallel work. No delegates, new persistent tasks/sessions,
or model switches. The design turn ended with the package unstaged and
uncommitted. ROOT reviewed it and subsequently released implementation and the
sole global local-heavy lease in a later explicit interrupt.

## Verification results

Design checks completed:

- `python3 scripts/list-docs.py validate`: passed, 386 decisions and 1627
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Task 01's local coverage preflight: actual four Markdown changes returned
  `exempt`, no errors; separately labelled SIMULATED planned Go scope returned
  `covered`, no errors, validating requirement/AC/design/manifest links only.
  The later implementation preflight reads actual tracked and untracked owned
  paths and requires `covered` with no errors once Go changes exist.
- `git diff --check`: passed; status shows only the four design-package files,
  unstaged and uncommitted. The CLI catalog discovers the new design.
- Remote main remains `805f90e7beb813e7d44f0902bb77540c14fcd193`.

An extra `python3 scripts/lint-spec-files.test.py` run had already passed all
36 tests before ROOT narrowed required checks. The script is unchanged; its
unit suite is not a required gate and will not be replayed.

Implementation checks completed after ROOT's later release:

- Independent full-renderer RED, Task 01's exact command: session 52096 joined,
  exit 1; 20 dollar cases failed for duplication, 8 plain/ordinary-character
  controls passed, package 0.099s. Production retained its qualified blob.
- Pre-fix context RED with the same Go bounds and the other three named test
  functions: session 63195 joined, exit 1; PATH dollar case failed, command and
  launchd controls passed, package 0.084s.
- Task 01's complete launcher package command: session 80984 joined, exit 0,
  package 18.678s with Go 1.26.0, race, trimpath, fts5, p=1, GOMAXPROCS=2,
  GOMEMLIMIT=512MiB, GNU 180-second timeout and ten-second kill grace.
- Task 01's canonical changed-Go complete-package lint: session 20709 joined,
  exit 0, `0 issues`, GOMEMLIMIT=3GiB/internal five minutes/outer 360 seconds.
  Lint was inadvertently started before the GREEN handle finished joining;
  both actual handles were retained and joined, and ROOT was informed. The
  remaining heavy commands ran serially.
- Catalog validation and all-spec lint passed again; actual coverage included
  all seven owned paths, including the untracked Go test/design/work order,
  and returned `covered` with no errors. No simulated input was needed.
- Missing worktree dependencies received one `pnpm install --frozen-lockfile`
  from `apps/`, using existing pnpm 9.15.9/Node 24.21.0; session 11053 joined,
  exit 0, 937 packages reused, none downloaded. No lockfile change.

Normal active hooks and publication follow these checks; their exact receipt
belongs to the delivery session. No existing service/config/manager changes,
physical Kandev launch, manager smoke, or private fixture replay occurred.

Required-CI follow-up: ROOT authorized only the existing task-list pagination
test's search-settlement synchronization after shard 24 exposed a pre-existing
debounce race. The hosted failure and network trace provide RED; the focused
guarded Chromium case passed (one worker, zero retries, 13.2s), followed by scoped
ESLint and Prettier checks. All commands joined with known owned groups absent.
Task 01 records the precise scope, initial zero-test selector correction, and
evidence limits. No product pagination or service code changed in this follow-up.

Actual conflict integration: main advanced to `9c24ab73ec`, independently adding
equivalent pagination synchronization. ROOT authorized an ordinary merge. Its
expected exit 1 identified only the pagination spec; resolution retained the
exact upstream version and all assertions. Upstream backend/web changes required
a fresh guarded build. The same single-worker, zero-retry Chromium case passed
(7.3s); native 53861 joined exit 0, owned group absent. Prior-head required CI
passed; the integrated head requires fresh hosted checks after publication.

## Risks

- A global dollar change would break the command context; the command control
  must stay separate from environment assertions.
- Disabling all percent escaping would corrupt literal paths; preserving trusted
  `%h` must not admit specifiers from detected prefixes.
- Existing wrong tests can conceal the defect; derive expectations from the
  authoritative systemd grammar and ROOT receipt, not current helper output.
- Portable renderer checks do not prove full installed service health. Keep
  process-boundary and installed-runtime claims separate.
