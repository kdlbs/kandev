---
name: debug-logs
description: Add debug logs (temporary console.log / structured Warn, or permanent namespaced loggers) to investigate or instrument runtime behaviour. Use whenever the user wants to add logs, log statements, console.logs, trace, instrument, or print runtime behaviour to debug a frontend or backend issue. Triggers include "add debug logs", "add some logs", "log this", "trace this", "instrument", "investigate why", "print", "console.log around". Temporary debug logs must be stripped before creating a PR; persistent ones (frontend `createDebugLogger`, backend tier-appropriate level) stay.
---

# Debug Logs

Add debug logs to investigate runtime issues or instrument recurring failure surfaces. There are **two distinct flavours** — pick the right one up front:

| Flavour | Use when | Lifetime |
|---|---|---|
| **Temporary** (`console.log` / `logger.Warn("[DEBUG] ...")`) | One-off investigation of a specific bug, with the user iterating in their terminal/browser. | **Strip before commit.** Never merge. |
| **Persistent — frontend** (`createDebugLogger` from `@/lib/debug/log`) | The data flow is one users (or future you) might want to inspect again — store hydration, WS dispatch, layout restore, executor compat, etc. | **Stays in code.** No-op in prod; opt-in via `make start-debug`. |
| **Persistent — backend** (`logger.Debug` / `logger.Info` at appropriate level) | Same idea on the backend — the existing structured logger already supports namespaces and `--debug` filtering. | Stays in code. |

**Use this skill any time you are about to add a `console.log`, `logger.Warn("[DEBUG] ...")`, or similar instrumentation.** Even if the user says just "add some logs", "throw a few logs in there", "trace this", or "instrument X", apply these rules.

If the user says "add logs to debug X", default to **temporary**. If they say "instrument X" or "add logs so future debugging is easy", default to **persistent**. When in doubt, ask.

## Rules

1. **Temporary logs are temporary.** Strip them before running `/commit` or `/pr`. Persistent ones stay.
2. Use a consistent, searchable prefix so logs can be found and (for temporary ones) removed easily — e.g. `[reorder-bug]`, `[WS-DEBUG]` for temporary; namespaced like `domain:aspect` for persistent.
3. **Always print every value inline as a string.** Browser DevTools and many terminal viewers collapse arrays/maps/nested objects (`Array(2)`, `{...}`) — the agent must serialise these into the log message itself, not pass them as additional arguments.
4. Prefer **one template literal** per `console.log` call (temporary) or **flat primitives** as logger args (persistent). Use `\n` and indentation only for multi-line layouts the user must read at a glance.

## Persistent — Frontend (`createDebugLogger`)

The frontend ships a namespaced debug utility at `apps/web/lib/debug/log.ts`:

```typescript
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debug = createDebugLogger("executor-compat");

debug("check", { agent: name, executor_type: type, ok: result, reason });
```

Key properties:
- **No-op in prod** — the factory returns a `() => {}` when `IS_DEBUG` is false, so calls cost nothing. Safe to leave in shipped code.
- **Active when** `NODE_ENV !== "production"` (dev), `NEXT_PUBLIC_KANDEV_DEBUG=true` (build flag), or `window.__KANDEV_DEBUG === true` (runtime, set by `make start-debug`).
- **Logfmt output** — `[namespace] message key1=value key2="value with space" key3={"nested":1}` — flat, grep-friendly, copy-pasteable.
- **Mirrored to the in-app log buffer** (`lib/logger/intercept.ts`) so debug lines also appear in Improve Kandev reports without extra plumbing.

### Conventions

- **Namespace** is `domain:aspect` so devtools console filters can narrow on either part (`executor-compat`, `executor-compat:specs`, `ws:dispatch`, `git-status:store`).
- **Register the namespace** in the cheat-sheet docblock at the top of `lib/debug/log.ts`. Future agents triage by reading that list — an unregistered namespace is invisible.
- **One log per event**, not per render frame. The factory is no-op in prod but allocates a closure per call in dev — still cheap, but per-render-frame logs spam the console and slow devtools.
- **Guard expensive arg computation** with `if (IS_DEBUG) { ... }`. The `debug()` call itself is free, but JS evaluates args first.

### ✅ Correct — flat primitives as a single object arg

```typescript
debug("loaded", {
  count: specs.length,
  ids: specs.map((s) => s.id).join(",") || "-",
});
// → [executor-compat:specs] loaded count=7 ids=gh_cli,claude-acp,codex-acp,…
```

### ✅ Correct — guard expensive aggregation

```typescript
if (IS_DEBUG) {
  debug("compute", {
    input: profiles.length,
    output: filtered.length,
    blocked: profiles.filter((p) => !ok(p)).map((p) => p.id).join(","),
  });
}
```

### ❌ Wrong — passing a nested object that won't flatten

```typescript
debug("compute", { profile, specs });   // logfmt JSON.stringifies, hard to read
```

Pre-extract the fields you actually care about (`profile.id`, `specs.length`, ...).

### ❌ Wrong — log per render

```typescript
function MyComponent() {
  debug("render", { ... });   // fires on every render — use useEffect or event handler
  ...
}
```

## Temporary — Frontend (`console.log`)

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

- [ ] Have you picked the right flavour — **temporary** (`console.log` / `[DEBUG] Warn`, stripped before PR) or **persistent** (`createDebugLogger`, register namespace, stays)?
- [ ] Does the prefix/namespace match what's already in the file (or what you agreed with the user) so all logs are greppable?
- [ ] Is every value a primitive at log time? If not, pre-format with `.map().join()`, `JSON.stringify`, `strings.Join`, or `fmt.Sprintf`.
- [ ] Temporary frontend: `console.log` (not `warn`/`debug`)? Persistent frontend: `createDebugLogger(...)`? Backend temp: `Warn` (not `Debug`/`Error`)?
- [ ] Is the call site the cheapest possible — one log per event, not per render frame? For persistent logs with expensive arg computation, guarded with `if (IS_DEBUG)`?
- [ ] Persistent: did you register the namespace in the cheat-sheet docblock at the top of `apps/web/lib/debug/log.ts`?

## Workflow

1. **Pick the flavour** (temporary vs persistent — see decision table at top).
2. **Add the logs** to the relevant code paths. For temporary, keep them as unstaged changes. For persistent, treat them as normal code (review, typecheck, commit).
3. **Let the user test** the app and report back with console/log output.
4. **Iterate** — add, move, or refine logs as needed based on findings. **If the user reports the values are unreadable (`Array(2)`, `Object`, `{...}`), the previous log violated rule 3; pre-format the value before re-running.**
5. **Fix the issue** once the root cause is identified.
6. **Strip temporary debug logs** before committing the fix. Commit the actual fix; persistent logs stay.

## Stripping Temporary Debug Logs

When the issue is fixed and the user asks to commit, remove all **temporary** debug logs first. **Do not strip `createDebugLogger` calls or backend `logger.Debug(...)` calls** — those are intentional persistent instrumentation.

```bash
# Find temporary frontend debug logs
grep -rn 'console.log("\[WS-DEBUG\]' apps/web/

# Find temporary backend debug logs
grep -rn '\[DEBUG\]' apps/backend/

# Or use the prefix agreed with the user
grep -rn '\[AREA-DEBUG\]' apps/
```

Verify no temporary debug logs remain in staged files before proceeding with `/commit` or `/pr`. If you accidentally added a persistent logger when the user wanted temporary (or vice versa), convert it before committing.
