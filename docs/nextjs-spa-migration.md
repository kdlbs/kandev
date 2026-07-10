# Next.js to Go-served SPA migration guide

Use this guide when rebasing or finishing an in-progress PR that was written
against the old Next.js web runtime. The target architecture is ADR-0020:
production serves a Vite React SPA from the Go backend, and Go injects the boot
payload needed for first paint.

## Target contract

- Production does not run a Next.js server or any Node.js web runtime.
- `apps/web` may still use Node tooling for Vite, TypeScript, linting, tests,
  and development.
- The Go backend serves the SPA shell, static Vite assets, APIs, WebSockets, and
  `/api/v1/app-state`.
- Go injects `window.__KANDEV_BOOT_PAYLOAD__` before React mounts. The payload
  contains route metadata, runtime config, and `Partial<AppState>` initial
  state.
- Data that was visible on first paint in a Next server component must move to
  the Go boot payload or a route bootstrap that can skip fetching when the store
  is already hydrated.

## Rebase workflow

1. Resolve conflicts by preserving SPA adapters and Go boot-state behavior.
2. Audit the rebased changes for new `next/*` imports, mocks, server component
   fetches, `redirect()`, `notFound()`, `cookies()`, and `headers()`.
3. For each new store read added by the incoming PR, decide whether it affects
   first paint. If it does, add that data to the Go boot payload.
4. Add or update focused tests before broad verification.
5. Re-run the Next audit command before handing off.

### Rebase loop checklist

Use this loop when bringing an older branch across the SPA migration boundary:

1. Capture the old base: `old_main="$(git rev-parse origin/main)"`.
2. Fetch and rebase onto the new `origin/main`.
3. Inspect only newly merged main commits: `git log --oneline "$old_main"..origin/main`.
4. For web files touched by those commits, scan for `next/*`, server-data,
   route-loading, and boot-payload regressions before changing code.
5. Migrate only when the rebased branch reintroduces old Next runtime patterns
   or conflicts with the Go-served SPA contract.
6. Run focused E2E for the touched area; broaden only when the touched area is
   shared routing, boot payload, or app shell behavior.

Useful audit commands:

```bash
rtk rg -n 'from "next/|from '\''next/|vi\.mock\("next/|next/link|next/navigation|next/image|next/dynamic|next/headers|next-themes|styled-jsx|eslint-config-next|next\.config' apps/web apps/packages/ui apps/pnpm-lock.yaml apps/package.json
rtk rg -n 'redirect\(|notFound\(|cookies\(|headers\(|StateProvider initialState|searchParams|params:' apps/web/app
```

The first command should have no production `next/*` imports or tests mocking
Next modules. Comments and unrelated variable names are acceptable only after
manual review.

## Import replacements

| Old import        | Replacement                      |
| ----------------- | -------------------------------- |
| `next/link`       | `@/components/routing/app-link`  |
| `next/navigation` | `@/lib/routing/client-router`    |
| `next/image`      | `@/components/routing/app-image` |
| `next/dynamic`    | `@/lib/routing/client-dynamic`   |
| `next-themes`     | `@/components/theme/app-theme`   |

When replacing `next/navigation` in tests, mock
`@/lib/routing/client-router`, not the old Next module.

If a component needs route params from a new dynamic SPA route, update
`paramsForPath` in `apps/web/lib/routing/client-router.ts` and add coverage in
`apps/web/lib/routing/client-router.test.ts`.

## Migrating server-side data

Old Next pages often did this:

- Fetch request-time data in `app/**/page.tsx` or `layout.tsx`.
- Pass it to a client component as props.
- Hydrate Zustand with `<StateProvider initialState={...}>`.
- Use `redirect()` or `notFound()` for route decisions.

Migrate that as follows:

1. Identify the exact store fields the page needs before first paint.
2. Add route classification or params in `apps/backend/internal/webapp/routes.go`
   if Go needs to distinguish the route.
3. Add the data to `apps/backend/cmd/kandev/boot_state.go`. Prefer backend
   services directly over self-calling HTTP APIs.
4. If an HTTP handler already returns the same data shape, extract a small DTO
   mapper in that backend package and reuse it from both the handler and the
   boot-state builder.
5. Keep JSON shape aligned with the frontend store/API types. If the existing
   API shape is snake_case, the boot payload should use the same shape unless
   the store already expects otherwise.
6. Update the SPA route/client bootstrap so it skips its client fetch when the
   store already has boot data.
7. Add tests for the Go mapper or boot builder, and a frontend test proving the
   client does not refetch when boot state is present.

Optional or below-the-fold data may still fetch after mount, but do not regress
routes that previously rendered useful data without an initial loading spinner.

## Routing and redirects

- Add browser route rendering in `apps/web/src/spa-routes.tsx`,
  `apps/web/src/settings-routes.tsx`, or `apps/web/src/office-routes.tsx`.
- Use `useRouter().replace()` for client-side redirect behavior.
- Move server-only gating that must happen before first paint into Go boot-state
  route logic or expose enough boot state for the SPA to render the correct
  initial screen.
- Replace `notFound()` with the route fallback pattern already used by the SPA
  route modules, or return an explicit load/error state from the client route.

## Verification checklist

Run focused tests for the files you touched, then at minimum:

```bash
rtk go test ./cmd/kandev ./internal/webapp
rtk pnpm --dir apps --filter @kandev/web typecheck
rtk pnpm --dir apps --filter @kandev/web lint
```

For data-hydration changes, include one backend test for the boot payload or DTO
shape and one frontend test that starts with hydrated store state.
