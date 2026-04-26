---
name: debug-logs
description: Add temporary debug logs to investigate issues. Use when the user needs to trace runtime behavior in the frontend or backend. Debug logs must be stripped before creating a PR.
---

# Debug Logs

Add temporary debug logs to investigate runtime issues. These logs are **never merged** — they must be removed before creating a PR.

## Rules

1. **All debug logs are temporary.** Strip them before running `/commit` or `/pr`.
2. Use a consistent, searchable prefix so logs can be found and removed easily.
3. Log object fields **inline** — do not log raw objects (they render as `Object` in the browser console and require expanding).

## Frontend (TypeScript)

- **Level:** `console.log` — not `console.debug` (hidden by default) or `console.warn` (noisy).
- **Prefix:** `[WS-DEBUG]` (or another `[AREA-DEBUG]` prefix agreed with the user).
- **Format:** Log fields inline as key-value pairs so they are visible without expanding.

### ✅ Correct

```typescript
console.log("[WS-DEBUG] subscribeSession", {
  sessionId,
  refCount: current + 1,
  sentMessage: shouldSend,
});
```

Console output:
```
[WS-DEBUG] subscribeSession {sessionId: 'abc', refCount: 2, sentMessage: true}
```

### ❌ Wrong — raw object renders as `Object`

```typescript
console.log("[WS-DEBUG] subscribeSession", session);
// Output: [WS-DEBUG] subscribeSession Object   ← useless without expanding
```

### ❌ Wrong — wrong log level

```typescript
console.warn("[WS-DEBUG] subscribeSession", { sessionId });  // ← use console.log
console.debug("[WS-DEBUG] subscribeSession", { sessionId }); // ← hidden by default
```

## Backend (Go)

- **Level:** `WARN` — stands out from normal `DEBUG`/`INFO` output without being an error.
- **Prefix:** `[DEBUG]` (or another `[AREA-DEBUG]` prefix agreed with the user).
- **Method:** Use the structured logger: `s.logger.Warn("[DEBUG] description", "key", value, ...)`.

### ✅ Correct

```go
s.logger.Warn("[DEBUG] handleTaskMoved entering",
    "task_id", taskID,
    "session_id", sessionID,
    "from_step", fromStepID,
    "to_step", toStepID,
)
```

### ❌ Wrong — wrong level

```go
s.logger.Debug("[DEBUG] handleTaskMoved", "task_id", taskID) // ← lost in noise
s.logger.Error("[DEBUG] handleTaskMoved", "task_id", taskID) // ← triggers alerts
```

## Workflow

1. **Add debug logs** to the relevant code paths. Do not commit them — keep them as unstaged changes.
2. **Let the user test** the app and report back with console/log output.
3. **Iterate** — add, move, or refine logs as needed based on findings. Still no commits.
4. **Fix the issue** once the root cause is identified.
5. **Strip all debug logs** before committing the fix. Only commit the actual fix.

## Stripping Debug Logs

When the issue is fixed and the user asks to commit, remove all debug logs first:

```bash
# Find frontend debug logs
grep -rn 'console.log("\[WS-DEBUG\]' apps/web/

# Find backend debug logs
grep -rn '\[DEBUG\]' apps/backend/

# Or use the prefix agreed with the user
grep -rn '\[AREA-DEBUG\]' apps/
```

Verify no debug logs remain in staged files before proceeding with `/commit` or `/pr`.
