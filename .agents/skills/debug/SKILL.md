---
name: debug
description: Diagnose Kandev bugs, running-instance issues, UI/browser failures, and runtime behavior. Use when the user reports unexpected behavior, asks to investigate, asks to add logs/instrumentation, or when a fix needs root-cause evidence before implementing. Triage first, gather evidence safely, then hand off to /fix or /tdd for code changes.
allowed-tools: Bash(curl:*) Bash(jq:*) Bash(npx:*) Bash(scripts/kandev-instances:*) Bash(scripts/kandev-logs:*) Bash(scripts/dev-isolated:*) Bash(scripts/kandev-kill:*) Bash(go:*) Bash(rg:*) Bash(grep:*)
---

# Debug

Diagnose efficiently and safely. Debugging produces evidence and a root-cause hypothesis; `/fix` turns that into a regression-tested patch.

## First: Create The Pipeline

Create a visible task list:

1. **Triage** - classify the bug and choose the cheapest faithful path
2. **Gather evidence** - targeted test, debug export/logs, browser state, or instrumentation
3. **Diagnose** - trace the failure to root cause
4. **Report or hand off** - summarize evidence and invoke `/fix` or `/tdd` if code changes are needed
5. **Clean up** - remove temporary logs, throwaway repro tests, isolated instances, and browser sessions

## Triage Gate

Pick one path before launching anything:

| Class | Signals | Reference |
|---|---|---|
| Backend logic | validation, dedup, data shaping, workflow routing, API/service behavior | `references/backend-repro.md` |
| Live instance | user has a running instance already misbehaving and you need read-only state/logs | `references/instance.md` |
| UI/browser | layout, focus, click flow, WS-driven UI, console/network behavior | `references/browser.md` plus `references/instance.md` |
| Needs logs | current evidence is insufficient and instrumentation is needed | `references/instrumentation.md` |

Rules:
- Triage before launching anything.
- Use logs and targeted tests before browser automation.
- Never mutate the user's live instance. Read-only debug export/logs are allowed; browser interaction must use your isolated instance.
- Tear down only what you started. Never `pkill kandev`.

## Evidence Strategy

Start with the cheapest faithful reproduction:

1. Backend logic: write a throwaway focused Go repro test against the real service path. If it reproduces, convert it via `/fix` or `/tdd`.
2. Live instance: use `scripts/kandev-logs <port> --export` or `--level error`; do not relaunch.
3. UI/browser: launch `scripts/dev-isolated --web`, drive `npx playwright-cli`, and correlate console/network state with backend logs.
4. Unknown: trace from the symptom backward through code and add temporary instrumentation only where it will split the search space.

## Reference Files

Load only the reference needed for the selected path:

- `references/backend-repro.md` - targeted Go repro tests and backend-first debugging.
- `references/instance.md` - instance discovery, isolated launch, logs/export, and teardown.
- `references/browser.md` - `npx playwright-cli` browser debugging against isolated instances.
- `references/instrumentation.md` - temporary vs persistent frontend/backend logging rules.

## When To Use Instrumentation

Use `references/instrumentation.md` before adding:
- `console.log`
- `logger.Warn("[DEBUG] ...")`
- `createDebugLogger(...)`
- backend `logger.Debug` / `logger.Info` for persistent diagnosis

Temporary logs must be stripped before `/commit` or `/pr`. Persistent instrumentation stays only when it has ongoing diagnostic value.

## Hand Off To Fix

When you can state:
- what fails,
- where it fails,
- why it fails,
- how to reproduce it,

then stop debugging and use `/fix` or `/tdd` to implement the regression test and patch.

## Final Report

Report:
- Bug class selected
- Evidence gathered
- Root cause or strongest hypothesis
- Suggested fix path and files
- Cleanup performed
- Any remaining unknowns
