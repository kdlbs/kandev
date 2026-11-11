---
id: "01-plugin-bootstrap"
title: "Plugin repository bootstrap"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.5
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 01: Plugin repository bootstrap

## Summary

Turn the template copy in `JnManso/kandev-plugin-azure-devops` into the Azure DevOps plugin skeleton, building against the sibling Kandev checkout on the standalone SDK paths. The repository is the user's own; the maintainer is not involved.

## In scope

- Identity rename in `manifest.yaml` (`id`, `display_name`, `description`, `author`, `categories: ["connector"]`, `repo_url`, `icon`), `go.mod`, `Makefile` (`BIN`, `VERSION`), UI registration.
- SDK migration: Go imports to `github.com/kdlbs/kandev/pluginsdk` with `replace => ../kandev/pluginsdk`; npm `@kdlbs/plugin-sdk` as `file:../kandev/apps/packages/plugin-sdk`; `.kandev-sdk-ref` set to the removal checkout commit; a `kandev` symlink to `kdlbs-kandev` in the task directory.
- Adopt `recipes/source-control/` as one slice (manifest fragment, `server/recipe.go`, `references.go`, `ui/register.ts`, `review-store.ts`).
- Bitbucket toolchain: `ui/src/*.ts` built with esbuild (React external), Vitest, Playwright packaged-host specs, `internal/` packages, `assets/icon.svg` + `NOTICE.md` (official mark, source, date, SHA-256, trademark).
- Delete template demo surfaces (page, toolbar action, `ping` webhook, `task.created` counter) and their tests.
- `README.md` rewritten: purpose and status, features, development against a sibling checkout, build/test/package/install, executor broker requirement, `npm install --prefer-online` tip. `CHANGELOG.md` reset to Unreleased. `LICENSE` holder. `.github/pull_request_template.md`. Keep `docs/repository-baseline.md`.
- CI and release workflows (`build.yml`, `ci.yml`, `release.yml`) on the new SDK paths.
- Code review: the owner installs the GitHub review apps they want on the repository (for example CodeRabbit, Greptile, Codex, Claude). The template's `ci.yml` and `build.yml` stay as the checks; branch protection on `main` is optional.
- GitHub repository description and topics (`kandev-plugin`, `azure-devops`).

## Out of scope

- Any Azure DevOps provider code (task 02).

## Acceptance

- `make test`, `make package`, and CI pass; `rg -i 'kandev-plugin-template|template plugin' --glob '!docs/repository-baseline.md'` is empty.
- A test PR runs the template CI and the installed review apps comment on it.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && make test && make package)
(cd ../JnManso-kandev-plugin-azure-devops-main && rg -i 'kandev-plugin-template|template plugin' --glob '!docs/repository-baseline.md'; test $? -eq 1)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/manifest.yaml`
- `../JnManso-kandev-plugin-azure-devops-main/go.mod`
- `../JnManso-kandev-plugin-azure-devops-main/Makefile`
- `../JnManso-kandev-plugin-azure-devops-main/package.json`
- `../JnManso-kandev-plugin-azure-devops-main/README.md`
- `../JnManso-kandev-plugin-azure-devops-main/.github/workflows/`
- `../JnManso-kandev-plugin-azure-devops-main/server/`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/`
- `../JnManso-kandev-plugin-azure-devops-main/assets/`

## Dependencies

None

## Risks

- Installing GitHub apps is an owner step in the browser; the agent prepares the files.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-05: Claude GitHub App installed by the owner through `/install-github-app`. PR #1 merged and added `.github/workflows/claude.yml` and `claude-code-review.yml`. Repository secret `CLAUDE_CODE_OAUTH_TOKEN` is present.
- Remaining for this task:
  - Limit `claude-code-review.yml` to `opened` and `ready_for_review`, so pushes do not spend the subscription. Re-reviews are requested with `@claude review`.
  - Optionally pin both actions to SHAs.
  - Rebase the local plugin worktree on `main` before the bootstrap commit.
- 2026-11-05, PR #1 evidence:
  - CodeRabbit posted a summary, a review, and 3 inline comments; its check passed.
  - Greptile posted a summary, a review, and 1 inline comment; its check passed.
  - The Claude review job ran but skipped itself. claude-code-action refuses to run on a PR that adds or changes its own workflow, because the file must match the default branch. Evidence that Claude reviews is still pending and comes from the next PR.
  - The template CI failed in `make audit-recipes`: `npm audit --audit-level=high` found 2 high and 2 moderate dev-dependency advisories (vitest/@vitest/mocker). This failure comes from the template and is unrelated to the bots.
- Fixes from the PR #1 review findings go in a small workflow-only PR before the bootstrap PR. claude-code-action skips any PR that changes its own workflow, so if these fixes rode in the bootstrap PR, that PR could not prove that the Claude review works. The fixes:
  - `claude-code-review.yml`: run on `opened` and `ready_for_review` only, and skip fork PRs (`github.event.pull_request.head.repo.full_name == github.repository`). Fork runs have no secrets.
  - Both Claude workflows: `actions/checkout` with `persist-credentials: false`.
  - `claude.yml`: `include_comments_by_actor: "JnManso,coderabbitai[bot],greptile-apps[bot]"`, so comments from other people cannot steer a write-capable run, while `@claude` can still act on the review bots' findings.
  - `npm audit fix` (or bump vitest) so `make audit-recipes` passes.
- 2026-11-05: plugin PR #2 (`ci: harden Claude workflows`) applies all four workflow fixes and `npm audit fix`, which clears the two high advisories; two moderate advisories remain and need the vitest major upgrade in the toolchain change. CI is green (head `3b20ce1`, merge state CLEAN). CodeRabbit and Greptile found nothing actionable. Claude skipped the PR as expected, because it changes its own workflows.
- 2026-11-05: bootstrap implemented locally on plugin branch `feat/bootstrap` (from `ci/claude-workflow-hardening`, uncommitted). A `kandev -> kdlbs-kandev` symlink in the task directory satisfies the sibling paths.
  - Identity: manifest id `kandev-plugin-azure-devops`, `api_version: 2`, version `0.1.0`, `min_kandev_version: "0.99.0"` (the SDK release floor in `pluginsdk/sdk-release.json`; raised at release), `categories: ["connector"]`, `repo_url`, `icon: assets/icon.svg`. Go module, Makefile `BIN`/`VERSION`, and `PLUGIN_ID` match; `server/manifest_test.go` enforces it.
  - SDK: `require github.com/kdlbs/kandev/pluginsdk v0.1.0` with `replace => ../kandev/pluginsdk`; `@kdlbs/plugin-sdk` from `file:../kandev/apps/packages/plugin-sdk`. Neither SDK is published yet. `.kandev-sdk-ref` is `56af766dd9e9571e00e3c948426d75bf1c88b5a9`, the upstream `main` tip and the base of the removal branch.
  - Recipe adopted as one slice: Go in `internal/sourcecontrol/` (with `manifest.fragment.yaml` and README), TypeScript in `ui/src/source-control/`. The fragment is merged into `manifest.yaml` in task 02 together with the adapters, so the manifest does not claim `azure_devops` before a provider exists.
  - Runtime: `internal/plugin.Runtime` served by `server/main.go`. UI: `ui/src/bundle.ts` + `plugin.ts`, built by esbuild (React external); `ui/bundle.js` is generated and ignored.
  - Removed template demos: `server/plugin.go` (ping webhook, `task.created` counter, config greeting), `config_schema` demo, `ui/bundle.js` page and toolbar action, `tests/ui-action.test.js`, `scripts/stable-host-smoke.mjs`, `tsconfig.recipes.json`.
  - Icon: Microsoft's official Azure DevOps service icon, unmodified, from the Azure architecture icon package V23; `assets/NOTICE.md` records source, date, SHA-256, terms, and trademark.
  - Verifiers ported from the Bitbucket plugin (id, API version, icon, inventory, checksum format). `verify-package.sh` also accepts the manifest that `plugin-pack -platform-only` re-serializes (unquoted, host executable only); new negative tests cover it.
  - CI: sparse checkouts add `pluginsdk` (and `apps/packages/plugin-sdk` in build); `build.yml` installs UI dependencies before `make build`; `base-floor` job removed; `audit-recipes` renamed `audit`. Vitest bumped to 4.1.12, which clears the two moderate advisories.
  - License: both plugin SDKs are AGPL-3.0-only, and the built-in Azure DevOps code ported in tasks 02 to 06 is AGPL-3.0 code from other Kandev contributors. The owner chose AGPL-3.0-only on 2026-11-05: `LICENSE` is Kandev's AGPL-3.0 text, `package.json` declares `AGPL-3.0-only`, and `NOTICE.md` keeps the template's MIT notice for template-derived portions.
  - README rewritten, CHANGELOG reset to Unreleased, `.github/pull_request_template.md` added. GitHub topics `kandev-plugin` and `azure-devops` set.
  - Verification: `make test` (Go, UI typecheck/build/Vitest 15 tests, verifier tests), `make check-format vet`, `make package`, `make verify-package`, `make verify-package-host`, `npm audit` (0 vulnerabilities), clean `npm ci --ignore-scripts` + `npm test`, `go mod tidy` stable, template-name `rg` empty.
- Open:
  - No separate bootstrap PR: tasks 01 to 07 accumulate on `feat/bootstrap` (to be renamed for the single plugin PR P1), rebased on `main` after PR #2 merges. P1's CI and the review apps' comments complete this acceptance.
