---
name: simplify
description: Simplify recently changed code — inline one-off abstractions, remove speculative code, reduce nesting, replace cleverness with clarity. Run after implementing a feature.
---

# Simplify

Post-implementation simplification pass. Review recently changed code and actively simplify it while preserving all behavior.

## Planner Entry

The planner may simplify small localized changes directly; delegate larger or
cross-component work. Code/test/config changes still require final Spark
`verify`. A simplify worker owns one packet and does not spawn workers.

The best code is code you don't have to write. The second best is code anyone can read.

## Available skills and subagents

- **`verify` worker** — The planner runs this after simplification to ensure all
  tests, lints, and typechecks still pass.

## Steps

### 1. Identify what to simplify

Run `git diff --name-only` (or `git diff origin/<base>...HEAD --name-only` for a branch) to get the changed files. Read each one.

### 2. Apply simplifications

Work through each changed file. Preserve behavior by inspection; do not run
tests, lint, typecheck, or full verification from the simplify assignment.
Report any verification concerns or focused checks the planner should delegate.

**Inline one-off abstractions:**
- Helper functions with a single call site — inline them
- Wrapper types that add no behavior — remove the wrapper
- Interfaces with a single implementation and no test mock — remove the interface

**Remove speculative code:**
- Unused function parameters or return values
- Config options that are parsed but never read
- "Reserved for future" scaffolding, empty extension points
- Feature flags with no toggle mechanism

**Reduce nesting:**
- Replace nested if/else with early returns (guard clauses)
- Replace nested ternaries with if/else or switch
- Extract deeply nested blocks into named functions

**Replace cleverness with clarity:**
- Dense one-liners that hide intent — expand to readable multi-line
- Overly generic code where a concrete implementation is simpler
- Abstractions that add indirection without reducing duplication

**Remove noise:**
- Comments that restate code (`// increment counter` above `counter++`)
- Redundant type annotations where inference works
- Empty error handlers, unused catch variables
- Leftover debug logging

### 3. Verify

Report the required final change-aware verification and any focused concerns to the planner;
do not run verification yourself. The planner delegates it to the `verify`
worker. If anything breaks, the simplification changed behavior and the
planner assigns the correction to a worker.

### 4. Summary

Report what was simplified:
- Files modified
- What was removed/inlined/simplified
- Lines removed (net)
- Verification concerns or recommended focused checks
