## Dev-first workflow

Before writing an E2E test, validate the feature works interactively using
`pnpm --dir apps exec playwright-cli` against a dev server. This gives a fast
feedback loop — code changes are picked up by hot reload in ~1-2 seconds, no
production rebuild needed. Once confirmed working, translate the interactions
into a proper E2E test.

### Start the dev environment

Multiple agents may run in parallel, so use random ports for dev servers and inspect the calculated backend/agentctl ranges with `ss` or `lsof` before starting; choose an unused `E2E_PORT_OFFSET` and never kill unrelated Kandev processes. Managed runner shards are isolated, but separate raw Playwright processes are not automatically safe: fixture ports are backend `18080 + E2E_PORT_OFFSET + workerIndex` and agentctl `30001 + E2E_PORT_OFFSET*1000 + workerIndex*200`; `--repeat-each` advances `workerIndex`, so nearby fixed offsets can overlap later.

```bash
OFFSET=$((RANDOM % 100))
BACKEND_PORT=$((19000 + OFFSET))
FRONTEND_PORT=$((14000 + OFFSET))
```

Start the backend:
```bash
E2E_TMP=$(mktemp -d) && mkdir -p "$E2E_TMP/.kandev" && \
printf '[user]\n  name = E2E Test\n  email = e2e@test.local\n[commit]\n  gpgsign = false\n' > "$E2E_TMP/.gitconfig" && \
HOME="$E2E_TMP" KANDEV_HOME_DIR="$E2E_TMP/.kandev" KANDEV_SERVER_PORT=$BACKEND_PORT \
KANDEV_DATABASE_PATH="$E2E_TMP/kandev.db" KANDEV_MOCK_AGENT=only \
KANDEV_MOCK_GITHUB=true KANDEV_DOCKER_ENABLED=false KANDEV_WORKTREE_ENABLED=false \
KANDEV_LOG_LEVEL=warn apps/backend/bin/kandev &
```

Start the dev frontend:
```bash
KANDEV_API_BASE_URL=http://localhost:$BACKEND_PORT VITE_KANDEV_API_PORT=$BACKEND_PORT \
pnpm --filter @kandev/web dev --port $FRONTEND_PORT &
```

### Validate with playwright-cli

```bash
pnpm --dir apps exec playwright-cli open http://localhost:$FRONTEND_PORT
pnpm --dir apps exec playwright-cli snapshot                    # see page structure and element refs
pnpm --dir apps exec playwright-cli click e5                    # interact using refs from snapshot
pnpm --dir apps exec playwright-cli fill e3 "test input"
pnpm --dir apps exec playwright-cli snapshot                    # verify result
```

### Fast iteration cycle

1. Make a code change in `apps/web/`
2. HMR picks it up in ~1-2 seconds
3. `pnpm --dir apps exec playwright-cli snapshot` or `pnpm --dir apps exec playwright-cli screenshot` to verify
4. Repeat until the flow works correctly

### Translate to E2E test

Once validated, write the Playwright test using project fixtures and page objects. The `playwright-cli` interactions map directly to Playwright API calls:

| playwright-cli | Playwright API |
|---|---|
| `playwright-cli click e5` | `page.getByTestId('...').click()` |
| `playwright-cli fill e3 "text"` | `page.getByTestId('...').fill('text')` |
| `playwright-cli snapshot` (verify element visible) | `expect(page.getByTestId('...')).toBeVisible()` |

Use `data-testid` selectors in the test (not snapshot refs), and wrap common flows in page objects.

### Capture PR evidence

After confirming the feature works, capture screenshots or a video as proof for the PR:

```bash
# Screenshots of key states
pnpm --dir apps exec playwright-cli screenshot --filename=apps/web/.pr-assets/feature-before.png
# ... interact to show the feature ...
pnpm --dir apps exec playwright-cli screenshot --filename=apps/web/.pr-assets/feature-after.png

# Or record a video walkthrough
pnpm --dir apps exec playwright-cli video-start apps/web/.pr-assets/feature-demo.webm
# ... perform the user flow ...
pnpm --dir apps exec playwright-cli video-stop
```

Create `apps/web/.pr-assets/manifest.json` so the `/pr` skill picks them up:
```json
{
  "assets": [
    {"name": "feature-demo", "file": "feature-demo.webm", "format": "gif", "caption": "Feature demo"},
    {"name": "feature-after", "file": "feature-after.png", "format": "png", "caption": "Result"}
  ]
}
```

### Final verification

Always verify against the production build before finishing — dev mode can hide boot-payload, asset-serving, or hydration issues:

```bash
pnpm --dir apps exec playwright-cli close
# Kill dev server and backend
make build-web
cd apps && pnpm --filter @kandev/web e2e:raw -- tests/path/to/test.spec.ts
```
