---
id: "08-docs"
title: "Documentation and guidance updates"
status: done
wave: 3
depends_on: ["06-makefile-cutover"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 08: Documentation and guidance updates

## Acceptance

- No doc, skill, or agent-guidance file tells a reader that `make dev` runs through
  `apps/cli/src/dev.ts` or the TypeScript launcher.
- The root `CLAUDE.md` "Tooling" section states that all four launch modes (`dev`,
  `start`, `run`, `service`) go through the Go binary, and that `apps/cli` is the npm
  publishing shim only.
- `docs/remote-cloud-environment.md` reflects that `make dev` needs a Go toolchain plus
  pnpm-for-Vite, and no longer needs `tsx`.
- `docs/specs/INDEX.md` lists the new spec.

## Verification

~~~bash
grep -rn -E "apps/cli/src|cli/src/dev.ts|pnpm -C cli|tsx src/cli.ts" --include='*.md' --include='Makefile' --include='*.yml' . | grep -v docs/plans/
~~~

Returns nothing outside historical plan records under `docs/plans/`, which are
implementation records and are intentionally not rewritten.

## Files

- `CLAUDE.md` (root) — "Tooling" section
- `docs/remote-cloud-environment.md`
- `docs/specs/INDEX.md`
- `docs/public/**` — audit for launcher/dev-setup references via `/docs-maintainer`
- Any `.agents/skills/**` or `AGENTS.md` hit from the grep above

## Inputs

- `CLAUDE.md` "Tooling" and "Maintaining This File" sections — the latter requires this
  update in the same PR as the code change.
- `apps/backend/CLAUDE.md` — check whether the launcher package warrants a mention now
  that it owns every entrypoint.

## Risks

- `docs/plans/{diagnostic-logging,port-collision-safety,feature-toggles-restart-action}/`
  reference `apps/cli/src/*` paths that will no longer exist. These are records of
  completed work, not live guidance — leave them alone and say so in the PR rather than
  rewriting history.
- Consider whether "the Go launcher owns every entrypoint; `apps/cli` is a publishing
  shim" is a durable enough boundary to warrant an ADR via `/record`. It arguably is,
  since it constrains where future launcher features may be implemented.
