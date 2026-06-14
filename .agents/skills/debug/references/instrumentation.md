# Debug Instrumentation

Use before adding any temporary or persistent debug logging.

## Choose The Flavor

| Flavor | Use when | Lifetime |
|---|---|---|
| Temporary frontend `console.log` | one-off investigation with user/browser feedback | strip before commit |
| Temporary backend `logger.Warn("[DEBUG] ...")` | one-off backend investigation | strip before commit |
| Persistent frontend `createDebugLogger` | recurring inspectable data flow | stays |
| Persistent backend `logger.Debug` / `logger.Info` | recurring backend diagnostics | stays |

If the user says "add logs to debug X", default to temporary. If they say "instrument X" or "make future debugging easy", default to persistent. Ask when unclear.

## Universal Rules

1. Temporary logs are temporary. Remove them before `/commit` or `/pr`.
2. Use a consistent searchable prefix, e.g. `[reorder-bug]` or `[WS-DEBUG]`.
3. Inline every value as readable text. Do not rely on collapsed objects like `Array(2)` or `{...}`.
4. One log per event, not per render frame or hot loop.
5. Guard expensive persistent debug argument computation with `if (IS_DEBUG)`.

## Frontend Persistent

Use `apps/web/lib/debug/log.ts`:

```typescript
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debug = createDebugLogger("executor-compat");

debug("check", { agent: name, executor_type: type, ok: result, reason });
```

Conventions:
- Namespace is `domain:aspect` when useful.
- Register the namespace in the cheat-sheet docblock in `lib/debug/log.ts`.
- Keep debug helper dependencies minimal. `IS_DEBUG` is true under vitest, so partial mocks must export any symbols debug paths touch.

Good:

```typescript
if (IS_DEBUG) {
  debug("compute", {
    input: profiles.length,
    output: filtered.length,
    blocked: profiles.filter((p) => !ok(p)).map((p) => p.id).join(","),
  });
}
```

Avoid:

```typescript
debug("compute", { profile, specs }); // nested, hard to read
```

## Frontend Temporary

Use `console.log`, not `console.debug` or `console.warn`.

```typescript
console.log(
  `[reorder-bug] sidebar:render sort=${sort.key}:${sort.direction} active=${activeId ?? "-"}\n` +
  `  inputOrder:\n    ${inputs.map((t) => `${t.id}|${t.title}|state=${t.state}`).join("\n    ")}\n` +
  `  outputOrder:\n    ${outputs.map((t) => t.id).join(", ")}`,
);
```

Flat primitive object args are acceptable:

```typescript
console.log("[WS-DEBUG] subscribeSession", { sessionId, refCount: current + 1, sent: shouldSend });
```

Avoid nested objects and arrays unless pre-formatted.

## Backend

Temporary backend logs:

```go
s.logger.Warn("[DEBUG] handleTaskMoved entering",
    "task_id", taskID,
    "session_id", sessionID,
    "from_step", fromStepID,
    "to_step", toStepID,
)
```

For slices/maps/structs, pre-format:

```go
s.logger.Warn("[DEBUG] panel order",
    "task_id", taskID,
    "panels", strings.Join(panelIDs, ","),
)
```

Avoid temporary `logger.Debug` because it can be lost in noise, and avoid `logger.Error` because it triggers alert-style interpretation.

## Strip Temporary Logs

Before commit/PR:

```bash
grep -rn 'console.log(".*DEBUG\\|console.log(`.*DEBUG' apps/web/
grep -rn '\\[DEBUG\\]\\|\\[.*-DEBUG\\]' apps/backend/ apps/web/
```

Do not remove intentional `createDebugLogger` or backend persistent `logger.Debug` / `logger.Info` instrumentation.
