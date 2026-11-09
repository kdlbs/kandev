---
created: 2026-11-05
status: completed
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
legacy_specs: []
---

# Implementation Plan: Restore weekly agent CLI updates

## Overview

Restore the weekly PR path and refresh the shipped agent CLI versions.
First remove test dependencies on fixed shipped versions. Then supply the bot's
delivery records and documentation-check dispatch. Finally validate Copilot's
candidate adapter and refresh the catalogue through the existing updater.

Agents owns this repair because it owns the trusted runtime catalogue and
provider compatibility. CI retains ownership of documentation coverage.
Existing requirements cover the defects. No new product requirement or ADR is
needed.

Related decisions retain authority:
[exact runtime versions](../../decisions/2026-08-12-validated-managed-runtime-version-selection.md),
[repository token](../../decisions/2026-09-04-use-repository-token-for-runtime-pin-prs.md),
and [native Copilot policy](../../decisions/2026-10-28-copilot-native-policy-authority.md).

## Evidence and root cause

- The workflow schedules Friday at 09:00 UTC. All six scheduled runs from
  September 25 through October 30 failed before the commit and PR steps.
- The [September 25 run](https://github.com/kdlbs/kandev/actions/runs/36146611665)
  expected `pi-acp@0.0.33` after the updater selected `0.0.34`.
- The [October 30 run](https://github.com/kdlbs/kandev/actions/runs/41080694814)
  failed fixed Codex/OpenCode expectations and Copilot plugin fixtures.
- Local targeted Codex/OpenCode tests pass with the tracked catalogue. A Go
  overlay containing npm's current stable versions reproduces failures across
  agents, settings controller, settings handlers, and lifecycle.
- At the original baseline, Copilot plugin adapters used an exact-version gate.
  Main later replaced that gate with observed capability and inventory checks.
- The repository allows Actions PR creation. The configured setting reports
  `can_approve_pull_request_reviews: true`. Credentials are not the observed
  blocker.
- The bot stages only the runtime JSON. `validateCoverage` rejects that diff
  because it lacks a changed work order. The bot also omits a documentation
  evaluator dispatch. This is a separate defect after PR creation.

## Scope

### In scope

- Catalogue-derived expectations for shipped defaults, with independent package
  and argument assertions.
- Explicit selected-version fixtures for Copilot plugin success cases.
- Repeatable validation with a temporary catalogue that changes every pin.
- Bot-generated delivery records and a trusted-main documentation dispatch.
- Current stable pins, with native evidence for any added Copilot adapter.
- Accurate adapter documentation and sanitized compatibility evidence.

### Out of scope

- Schedule changes, new credentials, auto-merge, or repository policy changes.
- A blanket documentation exemption or an automatic `no-docs-allow` label.
- Changes to runtime activation, OpenCode family migration, or running sessions.
- Global CLI installation, authentication setup, or plugin management.
- Enterprise-policy certification or changes to native policy authority.
- UI changes or browser tests.

## Technical approach

### Catalogue and fixtures

Use `DefaultManagedNPMRuntimeVersion` or its existing must-getter for assertions
about shipped defaults. Keep fixture versions for explicit operator selections
and migration tests. Copilot success fixtures install a fake selection store
that returns an explicit version. Native capability and format checks determine
plugin admission, including for previously unobserved versions.

### Weekly PR record

Add `.github/scripts/record-agent-runtime-pin-refresh.mjs` with fixture tests.
It receives the trusted before/after catalogues and writes the two fixed record
paths defined in the runtime design. It validates both inputs and records only
actual changes. The workflow saves the original catalogue before updating and
invokes the generator after its Go validation succeeds.

Stage only the catalogue and the two generated Markdown files. Export the
created/refreshed PR number through a step output. Preserve all six validation
dispatches and additionally dispatch `pr-docs.yml` on `main` with `pr_number`.
Test record coverage through the actual `validateCoverage` function.
The PR body follows the repository template and links the generated work order.

### Candidate versions

The registry returned these stable versions on 2026-11-05. Resolve them again
during implementation because release metadata can change.

| Package | Current | Candidate |
| --- | --- | --- |
| `@agentclientprotocol/claude-agent-acp` | 0.81.2 | 0.100.0 |
| `@agentclientprotocol/codex-acp` | 2.2.2 | 2.5.0 |
| `@openai/codex` | 0.154.0 | 0.166.0 |
| `@bex-co/muse-code-acp` | 0.7.0 | 0.11.0 |
| `@github/copilot` | 1.0.88 | 1.0.105 |
| `@google/gemini-cli` | 0.61.0 | 0.67.0 |
| `@opencode/cli` | 2.0.18 | 2.2.5 |
| `opencode-ai` | 1.18.32 | 1.19.5 |
| `pi-acp` | 0.0.34 | 0.0.34 |

Implementation resolved the same stable versions again on 2026-11-05 before
running the updater. The refreshed catalogue contains those exact values.
The later main-branch merge changed the prior Copilot default to `1.0.103`.
The generated delivery record compares the proposed pins with that new base.

### Copilot compatibility

| Context | Verification | Result required |
| --- | --- | --- |
| Current supported selected version | Hermetic discovery and process tests | Plugin fixtures work regardless of shipped default |
| Candidate exact managed package | Isolated native inventory, metadata, manifest, and repeated-mount fixtures | Record native compatibility evidence |
| Unobserved selected version | Capability and inventory controls | Admit compatible shapes without global fallback |
| Missing enterprise-policy fixture | Existing native-authority contract | Report untested policy without a new admission gate |

## Tests

- `AC-AGENTS-RUNTIME-UPDATES-001.5`, `.7`, `.9`: complete command and metadata
  assertions pass against both tracked and modified catalogue values.
- `AC-AGENTS-COPILOT-PLUGINS-001.1`, `004.3`, `005.4`, `005.8`: plugin success
  fixtures select an explicit runtime. Capability and format failures remain rejected.
- `AC-CI-PR-DOCS-001.3`, `.4`: generated records pass the existing validator.
  Catalogue-only and broken-reference fixtures still fail.
- `AC-CI-PR-DOCS-003.3`: workflow contracts require the documentation dispatch
  to use `main` and the generated PR number.
- Native adapter evidence follows the exact package and current supported
  metadata contract. It does not imply authenticated tool or policy coverage.

## Work orders

- [x] [Task 01: Make runtime tests independent of shipped pins](task-01-decouple-runtime-tests.md)
- [x] [Task 02: Complete weekly PR records and validation](task-02-complete-weekly-pr.md)
- [x] [Task 03: Validate and refresh stable CLI pins](task-03-refresh-stable-pins.md)

Execute these work orders sequentially in this session after an explicit
implementation request. No subagents or persistent follow-up tasks are
authorized.

## Verification results

Implemented and verified on 2026-11-05:

- The updater tests pass 9/9. The workflow contract tests pass 13/13, including
  executable stubbed-`gh` create, refresh, and dispatch paths. The PR
  documentation workflow contract passes 7/7, and action-pinning tests pass 9/9.
- The generator tests pass as part of an 18/18 combined updater and record run.
  Generated records pass the actual documentation coverage validator.
- The six-package suite passed with the tracked catalogue and with a temporary
  overlay that changed every pin, both with the `fts5` build tag. The pre-repair
  overlay reproduced the reported failures.
- The six-package suite plus `copilotplugins`, `internal/agentctl/server/utility`,
  and `internal/agentctl/server/adapter/transport/acp` passed with the refreshed
  catalogue and the `fts5` build tag.
- The isolated `copilot_native` inventory test passed for `1.0.105`, including
  exact package/binary identity, repeated mounts with a spaced path, local
  marketplace installation metadata, manifests, and adapter root resolution.
  Its command paths use the production process-tree and pipe-drain ownership
  helper, with a focused cancellation regression.
- The Claude ACP prompt-suggestion fallback now includes the inspected `0.100.0`
  package contract alongside `0.81.2`, with exact-version tests and no range
  expansion.
- `python3 scripts/list-docs.py validate` validated 431 decisions and 1920
  specifications. `python3 scripts/lint-spec-files.py --all` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed 63/63 and
  `node scripts/validate-public-docs.mjs` validated 55 pages.
- `bash scripts/lint-go-changed` passed for the changed Go paths, and
  `git diff --check` passed.
- The generated record paths contain the actual previous and proposed versions
  for the eight changed packages and the validation that preceded generation.

Merge-conflict reconciliation on 2026-11-06 preserved the newer capability-based
Copilot contract and the runtime pin refresh. The selected-runtime passthrough
fixture now asserts its explicit selection. The nine affected Go packages passed
with `go test -trimpath -tags fts5`, including lifecycle and Copilot plugins.
The updater and generator tests passed 18/18. The workflow contracts passed
13/13 and the documentation workflow contracts passed 7/7. Public documentation
tests passed 63/63, all 55 public pages validated, and web typecheck passed.
The generated delivery record compares the pins with the merged main baseline.
Before the subsequent non-conflicting main sync, the Docker, SSH, and Kubernetes
Codex executor E2E scenarios passed with one worker. The merge commit's normal
hooks passed, including Go lint. After that sync, web typecheck and the four
affected rich-output unit suites passed (91 tests).

## Risks

- A stable npm tag does not prove native protocol or plugin compatibility.
- A demonstrated Copilot capability or format incompatibility needs a reported
  decision and must not be hidden by a version-based fallback.
- Future automated PRs propose runtime changes for review. Passing unit tests
  does not automatically qualify every future Copilot adapter release.
- Generated records must change with the proposed versions and describe only
  checks that actually passed.
- Full workflow execution needs the changes on the selected workflow ref.
  Local validation does not prove remote PR creation or successful dispatch.
