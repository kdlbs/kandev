---
id: "04-integration-and-docs"
title: "Prove integration and document usage"
status: in_progress
wave: 4
depends_on:
  - "03-question-image-interface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-001.3
  - AC-TASKS-QUESTION-IMAGES-001.4
  - AC-TASKS-QUESTION-IMAGES-002.1
  - AC-TASKS-QUESTION-IMAGES-002.2
  - AC-TASKS-QUESTION-IMAGES-002.5
  - AC-TASKS-QUESTION-IMAGES-003.2
  - AC-TASKS-QUESTION-IMAGES-003.4
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.2
  - AC-TASKS-QUESTION-IMAGES-004.3
  - AC-TASKS-QUESTION-IMAGES-004.4
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 04: Prove integration and document usage

## Summary

Prove that images preserve clarification ownership and recovery across real
executor boundaries. Document the shipped authoring and operator workflow.

## In scope

- Add real Docker and SSH integration cases using the existing container fixtures.
  Create the image in the executor workspace. Fetch through the backend from a
  browser that cannot read that filesystem or contact executor localhost directly.
- Extend the image E2E suite with page reload, restart reconstruction, terminal
  history, missing files, source removal, and a different selected session.
- Prove timeout/detached answer, rejection, first-writer-wins, superseded-turn,
  choice-only, and image-free behavior. Add only missing regression cases.
- Update operator image-question guidance, the existing public usage guide, and
  tool prompt/schema guidance. Preserve the full hard-barrier wording.
- Record exact validation results and reconcile draft spec statuses only when
  all work orders pass. Do not mark an unavailable container run as passed.

## Out of scope

New product UI, provider capability expansion, broad local audits, new deployment
certification, snapshots, and publishing or GitHub operations.

## Acceptance

1. Actual Docker and SSH images render through authorized source-session transport.
   Remote failure cannot trigger host-path or localhost fallback.
2. Reload, restart, concurrent answers, rejection, stale turns, and late answers
   preserve the existing lifecycle with image-bearing questions.
3. Public docs and agent guidance describe the implemented contract. All prior
   work-order results are recorded before the plan becomes implemented.

## Focused validation cases

Add `e2e/tests/docker/question-image-previews.spec.ts` and
`e2e/tests/ssh/question-image-previews.spec.ts`. Use each directory's existing
executor setup and cleanup. Test a valid image, missing remote file, and revoked
or inaccessible source. Assert no absolute path or remote URL appears in browser data.

Extend `question-image-previews.spec.ts` with two source sessions containing different
bytes at the same relative path. Switch selected sessions and verify the question
still resolves its own source. Reload and restart using existing test fixtures.
Assert metadata survives even if bytes become unavailable. Do not promise unsent
draft persistence across reload. Verify a changed file is read freshly after reload.

Use existing resolver concurrency and retry suites for first-writer-wins evidence.
Add image-bearing fixtures where those suites would otherwise omit the new metadata.
Keep image failures separate from answer failure. Test inspection open during a
concurrent resolution or bundle replacement. Close stale inspection safely.

Run existing image-free clarification, choice-only, submission-failure, mobile,
and rich-output preview suites listed below. They verify this change does not
silently alter related clients or default image disclosure.

## Public documentation

Update `docs/public/agent-communication.md` with a focused operator how-to section.
Include an exact JSON example with two questions about one image, stable question
IDs, and keyed answers. Explain one-based generated option IDs without changing
the answer envelope. Recheck issue #4667 before documenting its current result.

Document PNG/JPEG support, 10 MiB and 16-million-pixel limits, four questions and
four distinct images per bundle, optional repository scope, no external URLs,
and live-file history limitations. Explain unavailable executor behavior and Retry.
Keep parent-question guidance separate.

Add a short operator instruction to `docs/public/use-kandev.md`: inspect the image,
close it, answer, and submit the bundle. Link the detailed authoring guide. Search
README and `docs/screenshots.md` for stale claims. Add screenshots only when existing
capture fixtures produce suitable evidence. Do not add a new public page.

Update `internal/sysprompt` guidance only as needed to advertise image references.
Retain the exact accepted-call question barrier and autopilot capability rules.

## Verification

Run from the repository root. Reuse Task 03's dependency installation. Run E2E
commands sequentially. Managed runs rebuild assets and enforce resource limits.

```bash
(cd apps/backend && go test ./internal/clarification ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification.spec.ts tests/chat/clarification-choice-only.spec.ts tests/chat/clarification-submit-failure.spec.ts tests/chat/rich-output.spec.ts tests/chat/rich-output-evidence.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts tests/chat/mobile-clarification.spec.ts tests/chat/mobile-rich-output-evidence.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/question-image-previews.spec.ts tests/ssh/question-image-previews.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use local `.github/scripts/pr-docs.cjs::validateCoverage` with the complete changed
file set and referenced documents. It must validate every work order's requirements,
acceptance criteria, design, and plan. Do not publish a GitHub status from this check.
Record concrete lint commands for any added Go or E2E files.

If Docker or SSH infrastructure is unavailable, record the exact failed command,
reason, and unverified case. Leave the affected work order incomplete. Do not
replace real-executor evidence with mocks while claiming the same coverage.

## Files likely touched

- New `apps/web/e2e/tests/docker/question-image-previews.spec.ts`
- New `apps/web/e2e/tests/ssh/question-image-previews.spec.ts`
- `apps/web/e2e/tests/chat/question-image-previews.spec.ts` and its mobile counterpart
- Existing clarification retry/concurrency tests only where image fixtures add coverage
- `docs/public/agent-communication.md`, `docs/public/use-kandev.md`
- `apps/backend/internal/sysprompt/sysprompt.go` and focused schema/prompt tests
- This plan and its work orders, paired requirement/design lifecycle fields

## Dependencies

Tasks 01 through 03. Read e2e fixture-state, cleanup, and resource-safety guidance
before runs. Apply docs-maintainer for public updates. No new UI preview is needed
because this task verifies UI-01 through UI-04 without changing their structure.

## Risks

A local file fixture cannot prove remote execution. Restart tests must use durable
messages, not an in-memory request stub. Existing test files may assume no images.

## Parallelism

Sequential. Remote suites share resource budgets, and package results have one owner.

## Inputs

All requirement IDs, the full system design, prior work-order results, existing
clarification tests, and current Docker/SSH fixtures.

## Results

Implementation and public documentation are present. This work order remains
**in progress** because real Docker/SSH execution and the full MCP handler suite
have not passed in this environment. Nothing was committed or published.

### Passing evidence

- Final focused frontend run: 97 tests. See Task 03 for its exact command.
- Full clarification and system-prompt packages pass. Full MCP server package and
  focused handler/retry tests pass. An explicit race run exercises the real REST
  and MCP answer entry points for both legacy and image-bearing bundles. A separate
  image-bearing detached-answer test verifies failed delivery, retry, rejection,
  and winner replay. See Task 01 for the commands.
- Desktop image flow: two tests passed, including backend restart, lazy history,
  changed/deleted files, full-size scrolling, keyed answers, and draft/choice/position
  restoration across 767px, 768px, and 1280px widths. Log:
  `/tmp/question-images-checks/kandev-run.e2e.g4MGHDK3.log`.
- Final combined desktop run: all 40 image, clarification, choice-only,
  submission-failure, and rich-output cases passed after the final provider-key
  correction. Log: `/tmp/question-images-checks/kandev-run.e2e.eD0G0INc.log`.
  Light and dark viewer captures, including the 767px boundary, were inspected.
- Final mobile suite: 15 tests passed. Log:
  `/tmp/question-images-checks/kandev-run.e2e.kz1iEI9t.log`.
- Source ownership uses a backend adapter test with two sessions and different
  bytes at the same relative path. Source removal uses the real process/API tests.
  Those are focused integration tests, not a browser-selected-session scenario.
- Public-doc tests and validation pass (48 published pages). Catalog validation
  passes (384 decisions, 1618 specifications). Specification lint passes. Local
  coverage validation against all 70 changed files covers all four work orders
  with zero reference errors. Relative links and `git diff --check` pass.
- Changed Go and frontend lint, formatting, types, and localization pass.

Browser commands used the managed host runner with `KANDEV_SERVER_HOST=127.0.0.1`,
`E2E_PORT_OFFSET=23`, and the host's Chromium shared-library paths. The temporary
Nix library setup is `/tmp/question-images-browser-env.sh`. The final source was
rebuilt by the managed runner; subsequent unchanged-artifact runs used `--no-build`.
Commands from the repository root:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification.spec.ts tests/chat/clarification-choice-only.spec.ts tests/chat/clarification-submit-failure.spec.ts tests/chat/rich-output.spec.ts tests/chat/rich-output-evidence.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts tests/chat/mobile-clarification.spec.ts tests/chat/mobile-rich-output-evidence.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node /tmp/check-question-image-package.cjs
git diff --check
```

### Incomplete checks

`go test ./internal/mcp/handlers` reached its default 10-minute package limit.
`go test -timeout 30m ./internal/mcp/handlers` also timed out, during SQLite fixture
setup for `TestHandleMessageTask_DispatchErrorAfterExistingSessionSwitchPreservesQueues`.
No image assertion failed in that run. Log: `/tmp/question-images-handlers-tests.log`.
A further `go test -timeout 10m ./internal/mcp/handlers` with a fresh RAM-backed
`TMPDIR` and `GOTMPDIR=/tmp` still timed out during coordinator fixture setup.
Log: `/tmp/question-images-handlers-tmpfs-tests.log`. The temporary directory was
removed. These attempts do not count as a passing full package run.

This exact container command stopped before executing tests:

```bash
KANDEV_E2E_CONTAINERS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project containers tests/docker/question-image-previews.spec.ts tests/ssh/question-image-previews.spec.ts
```

The managed preflight reported missing `apps/backend/bin/mock-agent-linux-amd64`.
Log: `/tmp/question-images-checks/kandev-run.e2e.5Nbm14z8.log`.
The installed Docker 29.7.2 client's `docker info` also failed because
`/var/run/docker.sock` does not exist. Log: `/tmp/question-images-docker-preflight.log`.
Both Docker and SSH fixtures require that daemon. They remain unexecuted. Current
agentctl client/API tests prove the common transport contract, not provider connectivity.
Windows and macOS binaries compile but were not run natively; see Task 02.

### Follow-through

Run the full handler suite on an environment with a suitable time budget. Build
the container fixture artifacts and run the two executor cases with a working
Docker daemon. Reconcile these results before marking this work order or the plan
complete. No further product-scope question is required. Issue #4667 remains open;
this implementation keeps its existing answer envelope and question barrier.
