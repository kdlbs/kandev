---
created: 2026-10-15
status: in-progress
requirements:
  - REQ-UI-INTEGRATED-CONTROL-001
acceptance_criteria:
  - AC-UI-INTEGRATED-CONTROL-001.1
  - AC-UI-INTEGRATED-CONTROL-001.2
  - AC-UI-INTEGRATED-CONTROL-001.3
  - AC-UI-INTEGRATED-CONTROL-001.4
system_design:
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Integrated control navigation plan

Implement the reviewed native navigation direction on current upstream. Scope:
one all-off typed runtime flag, picker entry, route-owned desktop/phone sidebar,
authorized workspace/coordinator lists, and existing current conversation reuse.

1. Register the flag using the existing profile/config/registry/frontend contract.
2. Add the route identity and one shared, lifecycle-scoped data provider.
3. Connect the existing picker, AppSidebar, and AppNavSheet to the same five-menu
   navigation and workspace/coordinator/conversation tree.
4. Resolve existing task/session identities without the conversation POST or
   automatic recovery; reuse the native renderer and session drafts.
5. Run focused existing flag checks, frontend contracts/type/lint/i18n checks,
   and permanent route, ownership, read-lifetime, lifecycle, and desktop/phone
   browser regressions. Hosted execution is required for the delivery review.

Work order: [Navigation and current conversation](task-01-navigation-and-current-conversation.md).

Remaining original feature work: full execution and provider/model dashboards,
historical conversation discovery, and native independent conversation relations
and lifetime. AI collaboration permissions and usage accounting are separate
dependencies only for their consuming operations. Publishing and installation
remain the existing owners' responsibilities.

Rollback is the flag's OFF state plus the previous normal workspace navigation;
no schema/data migration or runtime setting activation is part of this change.

## Historical contributor verification

The results below describe the original contribution and disposable probes, not
final delivery acceptance. Permanent route, hook, store/context, lifecycle, component
and desktop/phone browser regressions have been added during PR review. Ownership,
Azure cache isolation, unlink races and phone navigation corrections are covered
by those real-module tests; all current-head hosted gates and actual browser
results remain pending.

- Runtime flag registry/profile packages passed. The new identity has no active
  or retired collision, defaults OFF in prod/dev/e2e, and requires restart under
  the existing runtime registry contract. No installation was enabled.
- Frontend TypeScript passed with an invocation-only 8 GiB Node heap allowance;
  the default 4 GiB process exhausted its heap. Changed-file ESLint, the six
  existing i18n checks, `git diff --check`, and changed-Go lint passed.
- Existing frontend feature/route/coordinator coverage passed (32 tests), as did
  the existing native Quick Chat renderer coverage (26 tests). No permanent
  tests were added. Specification lint, document inventory validation, and the
  existing PR documentation coverage function passed on the actual changed paths.
- A disposable Chrome fixture rendered the actual navigation and native session
  renderer over synthetic HTTP/WebSocket transports. Desktop and phone kept the
  integrated sidebar/drawer while opening the original second-workspace task;
  the real workspace selection stayed unchanged. Original session drafts and
  runtime-session selection survived conversation switches. Missing sessions,
  denied coordinator lists, and OFF were checked. OFF issued no integrated reads;
  no conversation-open, prepare, resume, inference, or capability-discovery request
  occurred. This verifies browser behavior, not a live provider or installation.
- The full common-config package reported five Windows home-path fixture failures;
  the same failures were independently reproduced on unchanged upstream config
  source. They were not modified by this navigation work.

The existing renderer now accepts `loadSettings=false` for this surface, because
its usual settings load can trigger provider capability discovery. Other callers
keep the default behavior. The coordinator task origin is included in the frontend
wire type so the original task relationship can be checked without a cast.

## Embedded VCS scope

The native renderer originally read active-workspace VCS state even when an
embedded conversation belonged to another workspace. A task-bound context now
keeps GitLab list/availability/feedback/merge/unlink and Azure links on the
original workspace. GitHub uses a local task-ID read response with task/workspace
filtering and existing WebSocket invalidation, preserving the regular workspace's
global cache. Its status/feedback/merge/unlink and add-to-context session use the
same original scope. Ordinary callers retain their active-workspace defaults;
the chips were not hidden and the global workspace was not changed.

A focused Chrome fixture with connections only in workspace Beta passed: all
three VCS surfaces displayed Beta data, GitHub WS changes updated the chip,
foreign task/workspace associations were excluded, explicit GitHub/GitLab
merge/unlink requests used Beta, Alpha selection/cache and Beta draft stayed
unchanged, and no execution request occurred. Changed-file lint and a temporary
typecheck selecting the affected source graph passed. The previously valid
navigation/renderer, i18n, Go and full type checks were not repeated.

The public Get Started guide now documents the toggle, workspace picker, shared
phone navigation, original-conversation scope and the first view’s history and
conversation-creation limits. The existing public-docs validator passed for all
48 published pages. Integration read the VCS request result and rendered image;
44 HTTP requests had no wrong workspace, forbidden execution request or console
error, and retained the original active workspace and PR cache.
