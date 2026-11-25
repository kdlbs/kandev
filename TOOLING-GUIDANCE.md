## Tooling

- Run pnpm from `apps/`. Run web-only scripts from `apps/web/`, or use `pnpm --filter @kandev/web`.
- Backend: `make -C apps/backend test` or `build`. For local Go lint, run `bash scripts/lint-go-changed <changed-go-files>` from the repo root. Full lint is `make -C apps/backend lint`.
- Launcher: `dev`, `start`, `run`, and `service` use the Go binary `apps/backend/bin/kandev`. `make dev` runs `bin/kandev-launcher`.
- CLI: `apps/cli` contains the npm shim. Run its tests with `node --test`.
- Web: use `pnpm --filter @kandev/web dev`, `build:vite`, or `lint`. Run `pnpm run typecheck` from `apps/web`.
- Desktop: use `pnpm --filter @kandev/desktop build` or `e2e` from `apps/`. Run Rust tests from `apps/desktop/src-tauri`.
- E2E: run `pnpm e2e:run` or guarded `pnpm e2e:raw` from `apps/web`. Do not override worker limits or overlap full suites. The `containers` project requires `KANDEV_E2E_CONTAINERS=1` and a Docker daemon. See `apps/web/e2e/README.md`.
- Repository: `https://github.com/kdlbs/kandev`. Container image: `ghcr.io/kdlbs/kandev`.

### Worktrees

A fresh worktree does not share `apps/node_modules/`. Run `pnpm install --frozen-lockfile` from `apps/` before pnpm commands or commits. Missing dependencies also break the commit hook.
