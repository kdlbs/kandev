# Tracing

Capture detailed execution traces for debugging and analysis. Traces include DOM snapshots, screenshots, network activity, and console logs.

## Basic Usage

```bash
# Start trace recording
playwright-cli tracing-start

# Perform actions
playwright-cli open https://example.com
playwright-cli click e1
playwright-cli fill e2 "test"

# Stop trace recording
playwright-cli tracing-stop
```

## Trace Output Files

When you start tracing, Playwright creates a `traces/` directory with several files:

### `trace-{timestamp}.trace`

**Action log** - The main trace file containing:
- Every action performed (clicks, fills, navigations)
- DOM snapshots before and after each action
- Screenshots at each step
- Timing information
- Console messages
- Source locations

### `trace-{timestamp}.network`

**Network log** - Complete network activity:
- All HTTP requests and responses
- Request headers and bodies
- Response headers and bodies
- Timing (DNS, connect, TLS, TTFB, download)
- Resource sizes
- Failed requests and errors

### `resources/`

**Resources directory** - Cached resources:
- Images, fonts, stylesheets, scripts
- Response bodies for replay
- Assets needed to reconstruct page state

## What Traces Capture

| Category | Details |
|----------|---------|
| **Actions** | Clicks, fills, hovers, keyboard input, navigations |
| **DOM** | Full DOM snapshot before/after each action |
| **Screenshots** | Visual state at each step |
| **Network** | All requests, responses, headers, bodies, timing |
| **Console** | All console.log, warn, error messages |
| **Timing** | Precise timing for each operation |

## Use Cases

### Debugging Failed Actions

```bash
playwright-cli tracing-start
playwright-cli open https://app.example.com

# This click fails - why?
playwright-cli click e5

playwright-cli tracing-stop
# Open trace to see DOM state when click was attempted
```

### Raw CDP performance traces

For performance measurements that need raw Chrome trace events, use a CDP
session and `transferMode: ReturnAsStream`. `Tracing.tracingComplete` provides
the stream handle after `Tracing.end`; decode each `IO.read` chunk as base64
only when its `base64Encoded` flag is true, then close the stream. Guard the
entire operation so tracing and the CDP session are cleaned up even when the
measurement fails. Do not disable page script execution during the interaction
because that changes the work being measured:

```js
const client = await page.context().newCDPSession(page);
let tracingStarted = false;
let stream;
try {
  const complete = new Promise(resolve =>
    client.once('Tracing.tracingComplete', resolve),
  );
  await client.send('Tracing.start', {
    transferMode: 'ReturnAsStream',
    categories: 'devtools.timeline,v8.execute',
  });
  tracingStarted = true;
  // Run the measured interaction here.
  await client.send('Tracing.end');
  ({ stream } = await complete);
  for (;;) {
    const chunk = await client.send('IO.read', { handle: stream });
    const bytes = chunk.base64Encoded
      ? Buffer.from(chunk.data, 'base64')
      : Buffer.from(chunk.data);
    processTraceChunk(bytes);
    if (chunk.eof) break;
  }
} finally {
  if (tracingStarted) {
    try { await client.send('Tracing.end'); } catch {}
  }
  if (stream) {
    try { await client.send('IO.close', { handle: stream }); } catch {}
  }
  try { await client.detach(); } catch {}
}
```

In production code, use a bounded wait for `Tracing.tracingComplete` and keep
the cleanup guards when the timeout fires. Do not parse every chunk as base64,
and do not leave the CDP session attached after the trace.

### Analyzing Performance

```bash
playwright-cli tracing-start
playwright-cli open https://slow-site.com
playwright-cli tracing-stop

# View network waterfall to identify slow resources
```

### Capturing Evidence

```bash
# Record a complete user flow for documentation
playwright-cli tracing-start

playwright-cli open https://app.example.com/checkout
playwright-cli fill e1 "4111111111111111"
playwright-cli fill e2 "12/25"
playwright-cli fill e3 "123"
playwright-cli click e4

playwright-cli tracing-stop
# Trace shows exact sequence of events
```

## Trace vs Video vs Screenshot

| Feature | Trace | Video | Screenshot |
|---------|-------|-------|------------|
| **Format** | .trace file | .webm video | .png/.jpeg image |
| **DOM inspection** | Yes | No | No |
| **Network details** | Yes | No | No |
| **Step-by-step replay** | Yes | Continuous | Single frame |
| **File size** | Medium | Large | Small |
| **Best for** | Debugging | Demos | Quick capture |

## Best Practices

### 1. Start Tracing Before the Problem

```bash
# Trace the entire flow, not just the failing step
playwright-cli tracing-start
playwright-cli open https://example.com
# ... all steps leading to the issue ...
playwright-cli tracing-stop
```

### 2. Clean Up Old Traces

Traces can consume significant disk space:

```bash
# Remove traces older than 7 days
find .playwright-cli/traces -mtime +7 -delete
```

## Limitations

- Traces add overhead to automation
- Large traces can consume significant disk space
- Some dynamic content may not replay perfectly
