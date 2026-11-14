# External template migration handoff

Status: outstanding. This PR changes no external repository and sends no author
notifications. Begin migration only after the exact Go/npm SDK version and its
host mapping pass public release verification. Until then, keep each template's
tested Kandev commit pinned.

## Source evidence

Read-only inspection on 2026-10-19:

- General template: [`805d2018773bf3c6f594f5c4f9e1b2d07e71bc48`](https://github.com/kdlbs/kandev-plugin-template/tree/805d2018773bf3c6f594f5c4f9e1b2d07e71bc48).
- Coordinator template: [`7f45580a5a3d0406b5ad80b169e89544cdd22b61`](https://github.com/kdlbs/kandev-plugin-coordinator-template/tree/7f45580a5a3d0406b5ad80b169e89544cdd22b61).

Both use the old backend module replacement and npm `file:` SDK dependency.
The general template pins Kandev commit
`570600439036e81f8e9e1c63f15c4abce8a6c846` in `.kandev-sdk-ref`.
Both Makefiles invoke backend-owned `cmd/plugin-pack` from `KANDEV_SDK`.
SDK publication therefore does not by itself remove their packaging checkout.
Refresh source and instructions before making changes in either repository.

## Required edits in each repository

1. **`go.mod`, `go.sum`, and Go source.** Replace the old backend requirement
   with `github.com/kdlbs/kandev/pluginsdk v0.1.0` after that version is verified.
   Remove the old `replace`; update server, recipe, and test imports, including
   generated wire imports if used. Run `go mod tidy` and commit its result.
   The plugin module must not pull in the backend graph to obtain SDK types.
2. **`package.json` and `package-lock.json`.** Replace the SDK `file:` dependency
   with exact registry version `0.1.0`. Regenerate the lock with the repository's
   chosen npm, then verify `npm ci --ignore-scripts` outside a sibling layout.
   Check recipe subprojects for their own locks. Do not hand-edit integrity.
3. **`manifest.yaml` and recipe manifests.** Read installed release records.
   Set the minimum host to at least the highest installed SDK floor and retain
   an API version supported by each SDK. The first host version is still unknown;
   do not copy a fixture version or infer it from SDK `0.1.0`.
4. **Makefile and CI.** Add `sdk-check` using the public
   `kandev-sdk-check@v0.1.0` command, with the frontend path when applicable.
   Run it after installs and before build/package. Remove SDK-only sparse
   checkouts and sibling assumptions from compilation/typecheck jobs.
5. **`.kandev-sdk-ref`, packaging, and release workflows.** Inspect every use of
   the old ref before deleting it. Keep a separately named, pinned tooling ref
   and checkout for `plugin-pack`, including the new root `pluginsdk/` directory
   required by the backend's local replacement. Alternatively, provide and test
   equivalent package construction/verification in that separate change.
   Removing or replacing packaging tooling is not completed by this PR.
6. **README and repository agent guidance.** Document public exact pins, the
   compatibility command, deliberate upgrades, and the remaining packaging-only
   source requirement. Keep a clear distinction between SDK and plugin versions.

## Acceptance evidence for the external changes

Use a clean plugin checkout without an SDK sibling. At minimum run:

```bash
go mod tidy
go test ./server/... ./recipes/source-control/server/...
go vet ./server/... ./recipes/source-control/server/...
npm ci --ignore-scripts
npm run typecheck:recipes
npm run test:recipes
go run github.com/kdlbs/kandev/pluginsdk/cmd/kandev-sdk-check@v0.1.0 \
  --manifest manifest.yaml --frontend-sdk node_modules/@kdlbs/plugin-sdk
```

Run `npm run test:ui` for the general template. The inspected coordinator
`package.json` has no `test:ui` script: record that difference and run its actual
UI tests, or add appropriate coverage in the separate change. Do not record a
nonexistent command as passed.

Then run each repository's host/full packaging verification with the explicit
packaging-tool input, inspect archive contents/checksums, and smoke-test declared
surfaces against the stated minimum host. Capture CI results, dependency graph,
public resolution, and package evidence in each template PR. No such external
validation has run in this task.

Use the [dated plugin inventory](author-notification-inventory.md) for later
outreach. Refresh contacts and link the verified SDK release, public migration
guide, compatibility result, and both template PRs. Sending notices requires
separate authorization.
