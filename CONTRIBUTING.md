# Contributing to Kandev

Contributions are welcome! This document covers the basics.

## Community

Join our [Discord](https://discord.gg/gWdCPGcFCD) to ask questions, discuss ideas, or get help with your contribution before opening a PR.

## Important

You must understand the code you submit. You're welcome to use AI tools to help write code, but every PR will be reviewed by a human maintainer and the feature you're contributing should be manually tested. If you can't explain what your code does and why, it's not ready to submit.

## Contribution language

Use English for issues, PRs, documentation, specifications, plans, code
comments, and review discussion. Product localization values can use their
target language. Keep the surrounding explanation in English.

## Before opening a PR

Discuss a large architectural change in an issue before implementation or PR
creation. A large change includes a new subsystem, a public API or protocol
change, a persistence or data-model change, a new execution boundary, an
authentication or permission-model change, or a cross-cutting change across
subsystems.

Describe the problem, proposed direction, affected boundaries, alternatives,
and migration or compatibility risks in the issue. Wait until maintainers have
discussed the direction before opening the PR. Link the issue from the PR. If
an agent is preparing the change, it must stop and report missing discussion
instead of opening the PR.

### Documentation coverage

The `PR documentation coverage` status check uses a deterministic structural
policy. It does not classify the PR title or use AI to decide whether a change
needs documentation.

These changes pass without a delivery package:

- files under `docs/` or Markdown files;
- Go test files ending in `_test.go`, JavaScript or TypeScript test files with
  `.test.` or `.spec.` names, and files under `apps/web/e2e/`;
- one web translation catalog at `apps/web/src/locales/<locale>/<namespace>.json`;
- files whose basename is `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`,
  `go.sum`, or `Cargo.lock`.

For other changes, add or modify a work order at
`docs/plans/<initiative>/task-<NN>-<slug>.md`. The work order must link to its
sibling `plan.md`, requirement IDs, acceptance criteria, and system-design
documents. Those referenced contracts can already exist on the base branch.
Do not make cosmetic edits only to force an unchanged contract into the diff.

For example, a `fix:` PR that changes `apps/backend/internal/runtime/runtime.go`
still needs a linked work order. A PR that changes only `apps/web/e2e/tasks/foo.ts`
or `docs/operations.md` does not need one. A rename is checked at both its old
and new path.

If a maintainer decides that a small exception does not need a delivery package,
the maintainer can apply the exact `no-docs-allow` label. The label persists
across pushes and affects only this status. Removing it reevaluates the current
PR without requiring a new commit. The workflow never applies or removes the
label.

The check validates links and identifiers, not semantic completeness or the
order of planning and coding. Read the workflow summary for triggering paths,
accepted references, missing artifacts, and corrective steps. Maintainers can
use the workflow's manual retry with a PR number when an event needs a retry.

## How to Contribute

1. **Fork and branch.** Create a feature branch from `main`.
2. **Keep PRs small and focused.** Keep one logical change per PR. Split
   unrelated cleanup, refactoring, and feature work. Smaller PRs reduce the
   risk surface, make review easier, and reduce maintainer burden.
3. **Update public docs when behavior changes.** User-facing docs live in `docs/public/**`. If your change affects CLI commands, config keys, install/deploy flows, workflows, executors, APIs, screenshots, or user-facing terminology, update the relevant public docs in the same PR. See the [public docs contribution guide](docs/public/README.md) when editing navigation or adding a page.
4. **Test your changes.** Run `make fmt` first, then `make typecheck test lint` before submitting. Manually verify that your feature works end-to-end, and add screenshots or recordings to the PR if it has a UI component. **If your change touches any UI files (anything under `apps/web/`), you must add or update Playwright e2e tests in `apps/web/e2e/` to prevent regressions.** Run them with `make test-e2e`. See [docs/test_e2e_web.md](docs/test_e2e_web.md) for patterns and fixtures.

## Bug Reports

Search [existing issues](https://github.com/kdlbs/kandev/issues) first. If your bug isn't already reported, open one with:

- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, browser, agent type)

## Feature Ideas

Open an issue describing the feature and the problem it solves. Keep it concise.

## Code Quality

New code must pass the existing linters and tests:

```bash
make fmt        # Format Go and web code
make typecheck  # TypeScript type checking
make test       # Backend + web tests
make lint       # Backend + web linters
```

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0](LICENSE) license.
