---
id: "03-end-to-end-and-docs"
title: "Verify delivery and reconcile documentation"
status: in_progress
wave: 3
depends_on:
  - "02-delivery-and-default-activation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-INSTANCE-002
  - REQ-AGENTS-MCP-INSTANCE-004
acceptance_criteria:
  - AC-AGENTS-MCP-INSTANCE-002.2
  - AC-AGENTS-MCP-INSTANCE-002.5
  - AC-AGENTS-MCP-INSTANCE-004.4
  - AC-AGENTS-MCP-INSTANCE-004.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md
---

# Task 03: Verify delivery and reconcile documentation

## Summary

Prove valid MCP delivery through existing clarification and TUI browser flows.
Reconcile route documentation and record the
installed-provider release checks without treating mock results as provider proof.

## In scope

- Add a local TUI MCP context case to `terminal-agent.spec.ts`, using the
  generated file and `/e2e:mcp-context`. Reuse current terminal helpers.
- Run existing desktop/phone clarification flows in the e2e profile. Run the
  existing SSH TUI context case when containers infrastructure is available.
  Do not enable interactive questions for TUI agents.
- Extend mock URL-consumption tests with scoped HTTP/SSE URLs if needed.
  Keep the mock clients consuming their supplied endpoint, with no path guessing.
- Update the agent-attached routing paragraph in public `automation-and-mcp.md`
  and the transport subsection of the active package design. Preserve the
  external backend API's existing routes and third-party contracts.
- Update scoped agentctl `AGENTS.md` and `MCP_DELIVERY.md` for effective mode,
  helper-local endpoints, and delivery through the selected client.
- Perform and record Claude ACP, Codex, and Cursor checks from the plan's manual
  release gates. Keep unavailable checks pending with the exact missing prerequisite.
- Record exact verification outcomes, then reconcile draft specification status
  and plan status only after every linked acceptance condition is satisfied.

## Out of scope

No new rendered UI, localization, screenshots, general QA/review audit,
performance benchmark, or full browser suite. No commits, pushes, PR changes,
or contributor-branch mutations. Configured PR reviewers remain the later
semantic-review gate after separately authorized delivery.
Wrong-port/no-dispatch and port-reuse evidence stays in the focused Go tests.
Do not add a browser isolation spec that duplicates those cases.

## Acceptance

1. Desktop/phone ACP question flows resume the originating agent. Local TUI
   context identifies the current session through its actual delivered URL.
   Run the recommended SSH case when its infrastructure is available.
2. Documentation describes scoped defaults and legacy compatibility accurately.
   All document/catalog/coverage/whitespace gates pass, and no public external
   MCP route or existing authority policy changes.
3. Installed Claude, Codex, and Cursor outcomes, versions, and modes are recorded,
   including Cursor native approval. Any missing release gate keeps this order
   pending or in progress instead of being marked complete from mock evidence.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks
them. Run browser commands sequentially through the managed runner.

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent -count=1)
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/clarification.spec.ts tests/terminal/terminal-agent.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-clarification.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Recommended when the Docker/E2E prerequisites in `apps/web/e2e/README.md` are available:

```bash
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --shards 1 --project containers tests/ssh/passthrough.spec.ts -- --grep 'remote MCP uses the active task and session'
```

Missing infrastructure is a recorded skip and does not block acceptance. It
does not prove SSH E2E coverage. Do not bypass freshness or silently skip the case.

Run the repository documentation coverage validator against this package.
The source sentinel activates delivery validation for a documentation-only
planning diff. It does not claim that production code changed.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/builtin-mcp-instance-scoped-routes';
const files = fs.readdirSync(dir).filter(name => name.endsWith('.md'))
  .map(name => `${dir}/${name}`);
files.push('docs/specs/agents/requirements/builtin-mcp-instance-scoped-routes.md');
files.push('docs/specs/agents/system-design/builtin-mcp-instance-scoped-routes.md');
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/agentctl/server/api/server.go', status: 'modified' },
    ...files.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
if (!result.ok) process.exitCode = 1;
NODE
```

Record the manual procedure's observed results from the plan. Those observations
are additional release evidence, not substitutes for these exact automated checks.

## Files likely touched

- `apps/web/e2e/tests/terminal/terminal-agent.spec.ts`.
- Existing clarification, mobile clarification, and SSH passthrough specs only
  when a targeted assertion needs extension.
- `apps/backend/cmd/mock-agent/{mcp_client_test.go,tui_test.go}` if URL-consumption
  coverage needs extension. Existing production mock code needs no path rewrite.
- `docs/public/automation-and-mcp.md` (reference content).
- `apps/backend/internal/agentctl/{AGENTS.md,MCP_DELIVERY.md}`.
- `docs/specs/agents/system-design/builtin-mcp-management.md` (transport subsection).
- This package's requirements, system design, plan, and work-order results/status.

## Dependencies

[Task 02](task-02-delivery-and-default-activation.md) supplies default-on scoped
instances and all artifact delivery paths. [Task 01](task-01-scoped-helper-routes.md)
supplies route isolation and negotiation. Docker is optional for the recommended
SSH E2E run. Installed providers remain prerequisites for manual release evidence.

## Risks

- Tests that seed a question directly bypass endpoint delivery and prove little here.
- TUI question checks would alter an existing product eligibility rule.
- Fixture/global profile mutations can leak into neighboring browser cases.
- Installed Cursor approval behavior cannot be certified by a file fingerprint alone.
- Root/public route examples mostly describe external MCP and must remain intact.

## Parallelism

`sequential`

## Inputs

- Plan: Tests, E2E tests, Verification results, Documentation impact, Risks.
- Existing clarification, terminal-agent, and SSH passthrough browser cases.
- Scoped web/mock-agent `AGENTS.md`, `/e2e`, `/tdd`, and `/docs-maintainer`.
- The design's Authority and observability section for unchanged UI/gates.

## Results

Automated delivery and documentation work is in progress. The new local TUI
case reads context through the generated helper-scoped MCP URL and verifies the
active session. Public routing guidance, the package transport design, and
agentctl delivery guidance now describe scoped defaults and legacy compatibility.

Verification passed on 2026-11-16:

- `(cd apps/backend && go test -trimpath ./cmd/mock-agent -count=1)`.
- Chromium managed E2E, `tests/chat/clarification.spec.ts` and
  `tests/terminal/terminal-agent.spec.ts`: 36 passed, including the new TUI
  helper-instance URL context case.
- Mobile managed E2E, `tests/chat/mobile-clarification.spec.ts`: 16 passed.
- SSH managed E2E, `tests/ssh/passthrough.spec.ts --grep 'remote MCP uses the active task and session'`:
  1 passed using the `containers` project with the host runner and real Docker
  SSH fixture. The default containerized Playwright runner skipped this worker
  because it has no Docker socket; the host-runner invocation exercised it.
- `python3 scripts/list-docs.py validate`: passed, 453 decisions and 2016 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 passed; `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed;
  `node scripts/validate-public-docs.mjs`: passed, 55 published pages.
- Repository `validateCoverage` package check: passed with all three work orders
  and their requirement/design references accepted.
- `git diff --check`: passed.

Installed-provider release checks remain pending. `claude --version` reported
Claude Code 2.1.322 and `codex --version` reported Codex CLI 0.167.0, but their
question/approval flows were not run. `cursor` is not installed in this
environment, so Cursor configuration and native approval remain unverified.
Mock and browser E2E results do not satisfy those provider gates; keep this
order `in_progress` and the linked requirements/design draft until they run.
