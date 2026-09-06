---
created: 2026-09-05
status: done
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
legacy_specs: []
---
# Implementation Plan: Trusted Browser HTML File Preview

## Overview

Replace the QuickJS and virtual-DOM implementation with a one-click static
preview server in agentctl. Publish the current editor buffer as an in-memory
entry document. Serve relative workspace assets from disk. Desktop uses the
existing Browser panel. Mobile uses the focused file viewer.

The product owner selected the trusted Browser-panel model. Implementation and
documentation must state that previewing executes trusted workspace code and
must not claim hostile-content isolation.

## Scope

### In scope

- An ephemeral loopback static server owned by each agentctl instance.
- Bounded in-memory overlays for current, unsaved HTML buffers.
- Workspace-rooted relative asset serving with traversal and symlink-escape
  protection.
- A session-authorized backend publish endpoint and agentctl client contract.
- Existing session port-proxy routing for every supported executor.
- Desktop Browser-panel open, focus, reuse, and refresh behavior.
- A full-height mobile iframe with `Show code` and preserved editor state.
- Localized trusted-code, progress, error, and retry copy in all locales.
- Removal of the preview-specific QuickJS and direct parse5 dependencies, the
  virtual DOM, worker code, and obsolete tests.
- Public documentation and focused backend, frontend, E2E, and desktop checks.

### Out of scope

- An untrusted-content security boundary or dedicated preview origin.
- Framework builds, HMR, server-side routes, or package installation.
- Review-diff previews, source mapping, and new Browser-panel inspector features.
- Replacing explicit development-server commands for non-static applications.

## Technical approach

### Workspace preview server

Add a concurrency-safe manager to agentctl. It binds one loopback ephemeral port
per selected workspace or repository root. The manager publishes up to 32
current-buffer overlays of at most 5 MiB each. It stops all servers with the
agentctl instance. Exact overlay paths win over disk content. All responses use
`Cache-Control: no-store`.

The server resolves request paths with the existing repository path helpers and
rejects traversal, encoded traversal, and symlink targets outside the selected
root. It does not rewrite document content or emulate browser behavior.

### Session publish contract

Add a task-session endpoint that authenticates the session, ensures agentctl is
ready, bounds the request, and forwards `{repo, path, content}`. Agentctl repeats
validation and returns `{port, path, version}`. The frontend builds the existing
session port-proxy URL and appends the version as a cache buster.

### Responsive UI

Desktop keeps the source editor open and calls the existing Browser-panel action
with the preview URL. Repeated activation republishes and reuses the panel.
Mobile publishes through the same API but renders the URL in its focused viewer,
with Kandev-owned `Show code` and error controls outside the iframe.

Before execution, user-facing copy identifies the action as trusted workspace
code. The iframe uses the existing Browser-panel sandbox policy. The feature
does not add an isolation claim or browser capability filter.

### Migration and documentation

Delete the worker runtime, virtual DOM, Shadow DOM renderer, navigation
normalizer, and preview-only QuickJS/direct parse5 dependencies. Keep Markdown
behavior and explicit development-server controls unchanged. Update public
guidance to distinguish the zero-configuration static preview from application
development servers and to state the trust consequence.

## Acceptance evidence

| Acceptance criteria | Evidence |
| --- | --- |
| `.1`, `.2`, `.3`, `.7`, `.8`, `.9`, `.10` | Frontend API/component tests plus desktop and mobile E2E for current buffers, panel reuse, trust copy, source preservation, and recovery |
| `.4`, `.5`, `.11` | Agentctl server tests and browser E2E for native scripts, browser APIs, relative assets, bounds, path containment, eviction, and shutdown |
| `.6` | Backend handler/client tests plus existing session port-proxy regression coverage |

## Work orders

### Trusted Browser delivery

- [done] [Task 10: Build the agentctl workspace preview server](task-10-agentctl-workspace-preview-server.md)
- [done] [Task 11: Add the session publish contract](task-11-session-preview-publish-contract.md)
- [done] [Task 12: Replace the virtual runtime with Browser-panel UI](task-12-browser-panel-preview-ui.md)
- [done] [Task 13: Document and prove browser-fidelity preview](task-13-browser-fidelity-docs-and-e2e.md)

### Superseded implementation work

Tasks 01 through 04 describe the original scriptless prototype. Tasks 05
through 09 describe the QuickJS and virtual-DOM implementation. They are
retained for traceability but cancelled because the selected product contract
is native browser fidelity:

- [cancelled] [Task 01: Establish preview state and sandbox contract](task-01-preview-state-and-sandbox.md)
- [cancelled] [Task 02: Add responsive HTML preview surfaces](task-02-responsive-html-preview.md)
- [cancelled] [Task 03: Publish HTML preview guidance](task-03-public-html-preview-guidance.md)
- [cancelled] [Task 04: Prove responsive HTML preview flows](task-04-responsive-html-preview-e2e.md)
- [cancelled] [Task 05: Build the capability-free preview runtime](task-05-script-capable-preview-runtime.md)
- [cancelled] [Task 06: Integrate the runtime with preview state and renderer](task-06-preview-state-and-renderer.md)
- [cancelled] [Task 07: Wire responsive preview surfaces](task-07-responsive-preview-surfaces.md)
- [cancelled] [Task 08: Publish script-capable preview guidance](task-08-script-capable-preview-guidance.md)
- [cancelled] [Task 09: Prove script execution and isolation](task-09-script-capable-preview-e2e.md)

## Dependency order

```text
Task 10 agentctl server
        |
        v
Task 11 session contract
        |
        v
Task 12 responsive UI and runtime removal
        |
        v
Task 13 docs, E2E, and final verification
```

Tasks 10 and 11 establish the serving and authorization boundary before any UI
depends on it. Task 12 removes the old runtime only after the end-to-end URL can
be produced. Task 13 verifies the shipped flow and updates guidance after its
behavior is stable.

## Risks

- Same-origin Browser-panel content can exercise Kandev/browser authority. This
  is an accepted product tradeoff and must remain visible to users.
- Agentctl server teardown and overlay locking must cover stop, restart, and
  executor failure paths without leaking listeners or data.
- Proxy rewriting and capability cookies must preserve relative, root-relative,
  redirect, module, and fetch requests.
- Multi-repository roots and symlinks can accidentally widen filesystem access
  unless existing canonical path helpers are used consistently.
- Reusing a Browser panel must not replace unrelated manually opened content.
- Removing runtime packages changes the shared lockfile and generated license
  catalog. The Markdown renderer still owns transitive parse5 licenses, so the
  verification must distinguish those from the removed preview dependency.

## Completion gate

The package is ready to merge after all required checks pass. These checks cover
the backend, frontend, documentation, localization, E2E, desktop, dependencies,
licenses, and PR CI. Every review thread must have a disposition before merge.

## Verification results

- Agentctl API tests and race tests passed, including deterministic concurrent
  publish/read, per-root ports, and HEAD/405 method coverage.
- Full backend tests passed with the task-host internal config overrides cleared.
- Focused frontend tests passed: 13 files and 69 tests, including stale
  session completion guards.
- Agentctl client and task-handler tests passed for typed 400/413/503
  propagation, malformed responses, oversized requests, and unavailable
  sessions. The unused lifecycle forwarding layer was removed.
- Web typecheck, lint, i18n checks, i18n ratchet, license generation, and Vite
  E2E build passed.
- Public-doc validation passed: 61 tests and 43 published pages.
- Raw desktop Chromium and mobile Chrome preview suites passed: 2 tests each.
- Desktop shell E2E passed.
- Fresh desktop and mobile screenshots were captured, inspected, and compressed
  for the pull request.
- Reproduced and fixed the CI-only frontend failure in the saved-repository
  picker by synchronizing the Radix menu dismissal and asynchronous discovery
  assertions.
- Disabled Happy DOM main-frame and child-frame fixture navigation, and isolated
  Vitest-importable E2E utilities from Playwright runtime imports to remove the
  unhandled loopback `ECONNREFUSED` failures.
- The full frontend suite passed after the fix: 1,871 test files, 15,869
  passed, and 4 skipped, with no unhandled network errors.
- Re-ran desktop and mobile native preview E2E locally: 2 tests passed in each
  project. Web lint, direct web typecheck, i18n checks, and formatting checks
  also passed.
