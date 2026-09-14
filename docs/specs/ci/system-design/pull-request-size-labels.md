---
status: draft
system: ci
requirements:
  - REQ-CI-PR-SIZE-001
---

# Pull request size label system design

## Purpose and boundaries

This design defines the trusted GitHub Actions workflow that maintains one review-size label on each pull request.

The CI system owns the event contract, file filter, size limits, label definitions, permissions, and contract tests. GitHub remains the label source of truth.

The workflow reads pull request metadata only. It does not read file contents, check out pull request code, or change other review automation.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-CI-PR-SIZE-001` | [Event contract](#event-contract), [File classification](#file-classification), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/pr-size-label.yml` | Reads changed-file metadata and maintains the size labels. |
| `actions/github-script` | Runs pinned API orchestration from the trusted workflow without a checkout. |
| GitHub pull request files API | Returns the current changed-file paths with pagination. |
| GitHub labels API | Creates missing label definitions and changes pull request labels. |
| `.github/scripts/pr-size-label-workflow-contract_test.py` | Protects events, file rules, limits, mutations, concurrency, and permissions. |
| `.github/workflows/lint-action-pinning.yml` | Runs the contract test and the action-pinning checks. |

## Event contract

The workflow uses this event contract:

```yaml
on:
  pull_request_target:
    types: [opened, reopened, synchronize]
```

`pull_request_target` loads the workflow from the trusted base branch. It also gives fork pull requests the write permission required for labels.

The concurrency group contains the pull request number. The group uses `cancel-in-progress: false`.

The workflow serializes surviving runs for one pull request. `cancel-in-progress: false` keeps an active run, but GitHub can replace a pending run with a newer event and does not guarantee dispatch order. Each surviving run reads the current file list and label state instead of relying on event snapshots.

## File classification

The workflow paginates `github.rest.pulls.listFiles` with 100 entries per page. It uses only each response item's normalized `filename` value.

A counted file must start with `apps/`. It must also meet one of these rules:

- Its extension is `c`, `cc`, `cpp`, `css`, `go`, `graphql`, `gql`, `h`, `html`, `js`, `jsx`, `mjs`, `mts`, `proto`, `rs`, `scss`, `sql`, `ts`, or `tsx`.
- Its path matches `apps/web/src/locales/<locale>/<namespace>.json`.

The filter rejects these repository tooling directories: `apps/backend/internal/agentctl/server/api/scripts/`, `apps/backend/internal/webapp/embedded/generated/`, `apps/backend/scripts/`, `apps/web/e2e/scripts/`, `apps/web/generated/`, and `apps/web/scripts/`. It rejects any path containing a `node_modules` directory.

The known linter directories are `apps/backend/cmd/sqlguard/`, `apps/backend/internal/db/sqlguard/`, and `apps/web/eslint-rules/`.

The filter rejects these exact tool setting base names: `commitlint.config.mjs`, `eslint.config.mjs`, `eslint.e2e-sleeps.config.mjs`, `eslint.i18n.config.mjs`, `eslint.i18n.options.mjs`, `playwright.config.ts`, `postcss.config.mjs`, `prettier.config.mjs`, `stylelint.config.mjs`, `tailwind.config.ts`, `vite.config.ts`, `vitest.config.ts`, `vitest.monaco-editor.ts`, and `vitest.setup.ts`. A source or test file with a similar prefix, such as `vite-preload-recovery.ts`, remains counted.

The filter excludes image, font, and binary files with known asset extensions and files under these asset directories: `apps/backend/internal/notifications/providers/assets/`, `apps/desktop/src-tauri/icons/`, `apps/web/lib/assets/`, `apps/web/public/`, and `apps/web/src/assets/`.

The explicit application root and extension list exclude these files without separate path rules:

- `docs/**`, `.github/**`, and root `scripts/**` files.
- Markdown guidance and public documentation.
- Package manifests, dependency locks, and TypeScript settings.
- Dockerfiles, shell scripts, image assets, and generated binaries.

Tests use the same path and extension rules as production source. The workflow does not inspect additions, deletions, or changed-line totals.

The count maps to labels as follows:

| Counted files | Label |
| --- | --- |
| 0 through 10 | `small` |
| 11 through 50 | `medium` |
| 51 or more | `big` |

## Label definitions

The workflow owns three repository label definitions:

| Name | Color | Description |
| --- | --- | --- |
| `small` | `0E8A16` | Pull request changes 0-10 application files |
| `medium` | `FBCA04` | Pull request changes 11-50 application files |
| `big` | `D93F0B` | Pull request changes 51 or more application files |

The workflow creates a missing definition before it changes pull request labels. It does not replace the color or description of an existing label.

## Control flow

1. GitHub starts the trusted workflow for an allowed pull request event.
2. The workflow reads all available changed-file pages for the current pull request.
3. The workflow stops before mutations if the changed-file result can be incomplete.
4. The workflow filters the paths and calculates the target size label.
5. The workflow reads the three repository label definitions and creates missing definitions. If another run created a label after the read, a 422 `already_exists` response is re-read as success; other errors fail the job.
6. The workflow adds the target label if the pull request does not have it.
7. The workflow removes the other size labels if the pull request has them.
8. The workflow writes the counted-file total and final label to the step summary.

The workflow adds the target label before it removes stale labels. An API error therefore does not remove the last valid size label first.

## Failure and recovery

The GitHub files endpoint returns at most 3,000 files. A result with 3,000 files can be incomplete, so the workflow fails before a label mutation.

A changed-file API error also fails before a label mutation. The current size labels remain unchanged in both cases.

A label-definition or label-mutation error fails the job. A concurrent label-definition creation that returns 422 with `already_exists` is re-read and does not fail the run. A retry reads current GitHub state and converges on one target label.

The workflow adds the target before it removes stale labels. A partial mutation can temporarily leave two size labels, but it does not leave the pull request unlabeled.

Surviving serialized runs do not promise event order. Each run reads current file and label state, so a later completed run can converge on the current pull request diff even when GitHub replaces a pending run or dispatches events out of order.

## Security

The job declares only `pull-requests: write`. GitHub permits this permission for file reads, label creation, and pull request label changes.

The workflow uses a full commit SHA for `actions/github-script`. It does not use `actions/checkout`, shell execution, `eval`, or pull request text in executable source.

The workflow passes repository and pull request values as API parameters. It does not interpolate untrusted metadata into executable workflow code.

## Observability

The step summary shows the pull request number, counted-file total, and selected label. API errors remain visible in the workflow result.

The contract test covers the event list, permission block, pagination, file filter, limits, label definitions, mutation order, and trusted execution boundary.

## Related decisions

- [Count Application Files for Pull Request Size](../../../decisions/2026-09-09-count-application-files-for-pr-size.md)

## External contracts

- [GitHub REST API: List pull request files](https://docs.github.com/en/rest/pulls/pulls#list-pull-requests-files)
- [GitHub REST API: Labels](https://docs.github.com/en/rest/issues/labels)
- [GitHub Actions: `pull_request_target`](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request_target)
