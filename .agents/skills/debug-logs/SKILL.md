---
name: debug-logs
description: Add temporary debug logs (console.log / structured Warn) to investigate issues. Use whenever the user wants to add logs, log statements, console.logs, trace, instrument, or print runtime behaviour to debug a frontend or backend issue. Triggers include "add debug logs", "add some logs", "log this", "trace this", "instrument", "investigate why", "print", "console.log around". Debug logs must be stripped before creating a PR.
---

# Debug Logs

Add temporary debug logs to investigate runtime issues. These logs are **never merged** — they must be removed before creating a PR.

**Use this skill any time you are about to add a `console.log`, `logger.Warn("[DEBUG] ...`, or similar transient instrumentation.** Even if the user says just "add some logs", "throw a few logs in there", "trace this", or "instrument X", apply these rules.

## Rules

1. **All debug logs are temporary.** Strip them before running `/commit` or `/pr`.
2. Use a consistent, searchable prefix so logs can be found and removed easily (e.g. `[reorder-bug]`, `[WS-DEBUG]`).
3. **Always print every value inline as a string.** Browser DevTools and many terminal viewers collapse arrays/maps/nested objects (`Array(2)`, `{...}`) — the agent must serialise these into the log message itself, not pass them as additional arguments.
4. Prefer **one template literal** per `console.log` call. Use `\n` and indentation to lay out structured data so the user can read it without clicking to expand.

## Frontend (TypeScript)

- **Level:** `console.log` — not `console.debug` (hidden by default) or `console.warn` (noisy).
- **Prefix:** `[area-bug]` / `[AREA-DEBUG]` agreed with the user.
- **Format:** A single template-literal string. Inline every field. Pre-format arrays/objects with `.map(...).join(...)` or `JSON.stringify(...)` before interpolating.

### ✅ Correct — template literal, every value inlined

```typescript
console.log(
  `[reorder-bug] sidebar:render sort=${sort.key}:${sort.direction} active=${activeId ?? "-"}\n` +
  `  inputOrder:\n    ${inputs.map((t) => `${t.id}|${t.title}|state=${t.state}`).join("\n    ")}\n` +
  `  outputOrder:\n    ${outputs.map((t) => t.id).join(", ")}`,
);
```

Renders as readable plain text in the console — no clicking required, copy-pasteable, diff-friendly.

### ✅ Acceptable — flat object of primitives only

When every value is a primitive (string/number/bool/null), an object literal is fine:

```typescript
console.log("[WS-DEBUG] subscribeSession", { sessionId, refCount: current + 1, sent: shouldSend });
```

Renders as: `[WS-DEBUG] subscribeSession {sessionId: 'abc', refCount: 2, sent: true}`.

### ❌ Wrong — array/nested object collapses

```typescript
console.log("[reorder-bug] render", { tasks: tasks.map(toCompact), groups });
// Output: [reorder-bug] render {tasks: Array(2), groups: {...}}  ← unreadable
```

Fix: pre-stringify (`tasks.map(...).join("\n  ")`) and embed in a template literal.

### ❌ Wrong — raw object passed as second arg

```typescript
console.log("[WS-DEBUG] subscribeSession", session);
// Output: [WS-DEBUG] subscribeSession Object   ← useless without expanding
```

### ❌ Wrong — wrong log level

```typescript
console.warn("[WS-DEBUG] ...", { sessionId });  // ← use console.log
console.debug("[WS-DEBUG] ...", { sessionId }); // ← hidden by default
```

## Backend (Go)

- **Level:** `WARN` — stands out from normal `DEBUG`/`INFO` output without being an error.
- **Prefix:** `[DEBUG]` (or another `[AREA-DEBUG]` prefix agreed with the user).
- **Method:** Use the structured logger: `s.logger.Warn("[DEBUG] description", "key", value, ...)`. Slog renders each key-value pair inline, so primitives are fine as-is.
- **For slices/maps/structs**, pre-format with `fmt.Sprintf` / `strings.Join` so the value lands on the log line as readable text instead of `[]string{...}`-style verbose output.

### ✅ Correct — primitives as structured fields

```go
s.logger.Warn("[DEBUG] handleTaskMoved entering",
    "task_id", taskID,
    "session_id", sessionID,
    "from_step", fromStepID,
    "to_step", toStepID,
)
```

### ✅ Correct — pre-format collections inline

```go
s.logger.Warn("[DEBUG] panel order",
    "task_id", taskID,
    "panels", strings.Join(panelIDs, ","),
)
```

### ❌ Wrong — wrong level

```go
s.logger.Debug("[DEBUG] handleTaskMoved", "task_id", taskID) // ← lost in noise
s.logger.Error("[DEBUG] handleTaskMoved", "task_id", taskID) // ← triggers alerts
```

## Quick Checklist (apply before every debug log you add)

- [ ] Does the prefix match what's agreed (or already in the file) so all logs are greppable?
- [ ] Is every value a primitive at log time? If not, pre-format with `.map().join()`, `JSON.stringify`, `strings.Join`, or `fmt.Sprintf`.
- [ ] Frontend: `console.log` (not `warn`/`debug`)? Backend: `Warn` (not `Debug`/`Error`)?
- [ ] Is the call site the cheapest possible — one log per event, not per render frame?

## Workflow

1. **Add debug logs** to the relevant code paths. Do not commit them — keep them as unstaged changes.
2. **Let the user test** the app and report back with console/log output.
3. **Iterate** — add, move, or refine logs as needed based on findings. Still no commits. **If the user reports the values are unreadable (`Array(2)`, `Object`, `{...}`), the previous log violated rule 3; rewrite as a template literal before re-running.**
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
