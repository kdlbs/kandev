---
id: "06-external-links"
title: "Scoped native external-link handling"
status: done
wave: 5
depends_on: ["05-native-notifications"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 06: Scoped Native External-Link Handling

## Acceptance

- Desktop `http`, `https`, and `mailto` destinations open in the system handler.
- Unsupported schemes, malformed URLs, credentials-in-URL surprises, and non-owned command inputs
  fail closed.
- Loopback Kandev routes, downloads, and blob URLs retain their existing in-WebView behavior.
- Direct external `window.open`/anchor call sites use one shared helper without changing ordinary
  browser behavior.
- Help and release-note menu links use the same restricted path.

## Files Likely Touched

- `apps/desktop/src-tauri/src/main.rs` and a scoped opener module
- `apps/desktop/src-tauri/Cargo.toml` and capabilities
- New external-link helper under `apps/web/lib/desktop/`
- Existing external-link and `window.open` call sites under `apps/web/`
- Focused URL classification and integration tests

## Verification

```bash
cd apps/desktop/src-tauri && rtk cargo fmt --check && rtk cargo test
cd apps/web && rtk pnpm run typecheck
cd apps/web && rtk pnpm test external
cd apps && rtk pnpm --filter @kandev/web lint
```

## Output Contract

Update this task to `done` and check its plan item only after scheme allowlisting and loopback
retention are covered by tests.
