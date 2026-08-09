# Request Mocking

Intercept, mock, modify, and block network requests.

## CLI Route Commands

```bash
# Mock with custom status
playwright-cli route "**/*.jpg" --status=404

# Mock with JSON body
playwright-cli route "**/api/users" --body='[{"id":1,"name":"Alice"}]' --content-type=application/json

# Mock with custom headers
playwright-cli route "**/api/data" --body='{"ok":true}' --header="X-Custom: value"

# Remove headers from requests
playwright-cli route "**/*" --remove-header=cookie,authorization

# List active routes
playwright-cli route-list

# Remove a route or all routes
playwright-cli unroute "**/*.jpg"
playwright-cli unroute
```

## URL Patterns

```text
**/api/users           - Exact path match
**/api/*/details       - Wildcard in path
**/*.{png,jpg,jpeg}    - Match file extensions
**/search?q=*          - Match query parameters
```

## Advanced Mocking with run-code

For conditional responses, request body inspection, response modification, or delays:

### Conditional Response Based on Request

```bash
playwright-cli run-code "async page => {
  await page.route('**/api/login', route => {
    const body = route.request().postDataJSON();
    if (body.username === 'admin') {
      route.fulfill({ body: JSON.stringify({ token: 'mock-token' }) });
    } else {
      route.fulfill({ status: 401, body: JSON.stringify({ error: 'Invalid' }) });
    }
  });
}"
```

### Modify Real Response

```bash
playwright-cli run-code "async page => {
  await page.route('**/api/user', async route => {
    const response = await route.fetch();
    const json = await response.json();
    json.isPremium = true;
    await route.fulfill({ response, json });
  });
}"
```

### Hold an upload request while asserting busy state

For multipart or large-upload pending-state tests, match one exact endpoint and
use a deterministic route handler. Resolve a `requestStarted` promise when the
handler runs, assert the busy contract, then release a promise that lets the
handler `route.fulfill` (or use `route.abort` for the failure path) and assert
recovery. Do not hold `route.fetch()` across competing upload handlers; that can
double-handle a route or corrupt gzip/body streams.

```typescript
let markStarted!: () => void;
let release!: () => void;
const requestStarted = new Promise<void>((resolve) => { markStarted = resolve; });
const settle = new Promise<void>((resolve) => { release = resolve; });
await page.route("**/api/uploads/exact-endpoint", async (route) => {
  markStarted();
  await settle;
  await route.fulfill({ status: 200, json: { ok: true } });
});
await triggerUpload();
await requestStarted;
await expect(uploadButton).toHaveAttribute("aria-busy", "true");
release();
await expect(uploadButton).not.toHaveAttribute("aria-busy", "true");
```

### Simulate Network Failures

```bash
playwright-cli run-code "async page => {
  await page.route('**/api/offline', route => route.abort('internetdisconnected'));
}"
# Options: connectionrefused, timedout, connectionreset, internetdisconnected
```

### Delayed Response

```bash
playwright-cli run-code "async page => {
  await page.route('**/api/slow', async route => {
    await new Promise(r => setTimeout(r, 3000));
    route.fulfill({ body: JSON.stringify({ data: 'loaded' }) });
  });
}"
```
