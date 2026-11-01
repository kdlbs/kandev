# Issue #4370 recovery

Recovery date: 2026-10-20. This note records evidence, not instructions to execute
against the source instance.

## Source identity and access

- Source: `http://100.105.155.17:9998`.
- Task: `455b8417-0979-4657-a7f0-800509852d12`,
  “Extend terminal shortcuts to passthrough sessions”.
- Session: `d7a31523-17f8-4fe9-8ea5-6ad653fd697e`. The session-list response
  reports exactly one session. All 347 unique messages were read in four pages.
- Identity evidence: the task description contains the exact
  [issue URL](https://github.com/kdlbs/kandev/issues/4370), the passthrough request,
  and the separate Quick Terminal request. Message
  `6ae27817-5f46-4456-b0bc-3a1a12aa29bf` repeats the original request.
- Current recovery task: `03a000c3-d50a-4e06-8ccb-aaa0a1c12d38`.
  Current session: `2bc3eb52-fcb9-4356-8c2d-463357238318`.

The source root and APIs returned HTTP 200. Network sandbox access failed first.
Approved network access succeeded. No credential was required or printed.
Only HTTP GET and stored-data WebSocket read actions were used.

Routes were discovered from repository handlers. Reads covered task/session
metadata, all conversation pages, turns, 179 stored command outputs, the current
task plan, all four returned plan revisions, plan comments, and walkthrough.
The file and Git read handlers call `GetOrEnsureExecution`. Those handlers were
not invoked because they could start a runtime. No source task, service,
repository, or session was restarted or changed.

## Approval and scope evidence

1. Message `55a4d4c9-6310-44b8-b348-868986d9efcf`
   (2026-10-09 11:05:45 UTC) handed off requirements, system design, plan,
   and one work order for `mobile-passthrough-navigation`.
2. User message `abf90486-4b74-44c8-b467-3af3293e1dae`
   (2026-10-09 11:08:00 UTC) states: “i deleted the quick terminal one, i had
   actually already implemented that. but go for it on the rest”.
   This is the later implementation authorization for the reviewed package.
3. The saved plan records that authorization and the implementation phase.
   Quick Terminal repair remains excluded. Native implementation delegation
   and publication were not authorized.

The separate Kubernetes investigation is task
`1dd6f24e-8788-4656-8db0-201a80609d9e`, session
`2cbcdab1-9797-4636-9606-8d096a61be4c`. Its reports are preserved as peer evidence
in the source conversation. Its task metadata links merged PR #4485. That
investigation is not part of this feature's implementation.

## Recovered artifacts and reliability

Private local snapshots are under `.kandev/recovery/4370/`. They include:

- The stored plan, four revisions, plan comments, and walkthrough responses.
- All 347 conversation messages, sanitized for credentials, plus selected excerpts
  with message IDs, timestamps, and author types.
- Selected historical validation outputs with their original message provenance.
- Eight extracted artifact snapshots and a SHA-256 manifest. These contain the
  four design-package files, the phone wrapper and its tests, the direct PTY
  sender, and the mobile browser tests.

Artifacts were extracted as text from stored command output. They were not
recreated by executing historical commands. The package files came from output
105 and 174. The wrapper and sender came from output 216. Test snapshots came
from outputs 151 and 158. The snapshots have different historical timestamps:
some tests precede later corrections. Preserve the snapshots and reconcile the
working copies rather than claiming an exact final-worktree restoration.

The full raw acquisition is under `/tmp/kandev-4370-recovery/`. It is temporary
and excluded from deliverables because logs can contain unrelated runtime data.
The sanitized conversation, excerpts, and artifact manifest retain relevant provenance.

The original worktree was `/workspace`, with base branch `main`. The final
file inventory reports uncommitted changes and no feature commit or PR.
No feature branch SHA was established. Do not use the Kubernetes child's
checkout SHA as this feature's base.

## Last reliable historical state

Message `2c3a8c60-049c-4633-8520-2980fbb4e417`
(2026-10-09 11:32:35 UTC) reports implementation complete and uncommitted.
It reports 116 unit tests and 39 browser tests passing, plus typecheck,
lint, localization, and documentation checks. Stored command outputs support
the test runs. These are historical results, not validation of this recovery.

Later messages concern preview hosting and port-forward failures. The final
user message, `5e6bb009-bda8-4a71-945f-938e9a5ec8ba`, reports a tunnel proxy
error. Session metadata records a failed replacement Kubernetes pod:
node affinity, topology spread, and taint constraints prevented scheduling.
No evidence establishes a later feature-code change.

Physical iPhone Safari and Android Chrome keyboard testing was explicitly
unavailable in the original work. Screenshot paths were recorded, but their
image bytes were not available from stored command output.

## Reconciliation and remaining work

The isolated workspace started at `595df36ed5c1cf02fd90cb972b623ad6edc53df8`.
The verified remote main was `e3ef99a98f8d340f2447650cb4318b00977eb44e`.
The clean local branch was fast-forwarded to that commit.

Fetch reported an unrelated damaged shared ref,
`refs/heads/feature/lead-pr-4371-to-merg-eeb`. The requested main objects were
available after transfer, and the explicit fast-forward succeeded. No unrelated
ref was repaired or deleted.

Main includes partial phone navigation from PR #4521 (`76954ff53ea`):
`AgentTerminalInput` registers a sender and the mobile footer shows a fixed bar.
The recovered design adds local socket ownership, readiness gating, an inline
bar, composer/comments suppression, and modifier cleanup. Preserve main's
ended-session recovery rendering and existing desktop behavior.

The recovered implementation was adapted into the existing `AgentTerminalInput`.
No duplicate wrapper was introduced. The shared direct-PTY helper moved from
settings to `lib/terminal`. The task-shell footer and ended-session recovery path
remain intact. Existing translations supply all control labels.

The initial local-validation checkpoint left changes uncommitted. The user later
authorized a branch push and an isolated test instance on 2026-10-20.
No PR or GitHub message was requested.

## Local validation

- TDD: the new unmount test failed against main because Ctrl remained latched.
  The new composer browser test also failed against main because its bar remained
  visible. Both failures exercised the recovered behavior gaps.
- Unit/component suite: 118 tests passed across seven files.
- Changed-file ESLint, formatting, localization, and TypeScript checks passed.
  Typecheck needed an 8 GiB heap after the default 4 GiB run exhausted memory.
- Specification catalog/lint, public documentation validation, its test, local
  PR documentation coverage, and `git diff --check` passed.
- Mobile browser suite: all 27 tests passed with retries disabled.
  Log: `/tmp/kandev-run.e2e.N7IQJhEJ.log`.
- Desktop browser suite: all 12 tests passed with retries disabled.
  Log: `/tmp/kandev-run.e2e.hGhzsYEp.log`. This includes physical keyboard input,
  the 767/768px fine-pointer boundary, and Quick Terminal regressions.
- Inspected the connected, simulated-keyboard, and composer phone captures.
  The key bar reserves space below terminal output and clears the keyboard area.
  The composer capture confirms that raw controls are absent. Captures and their
  SHA-256 manifest are in `.kandev/recovery/4370/local-validation/`.

The browser environment needed temporary setup outside the repository. The
inherited runtime config bound the isolated fixture to a non-loopback address.
The runner removes that inherited config and binds its disposable backend to
`127.0.0.1`. Chromium also needed installed Nix library paths and a temporary
Roboto Mono font from the official Google Fonts repository. The original font
fallback wrapped expected text inside terminal lines. Correcting the environment
made the existing composer prompt/attachment tests pass without changing them.
The new modifier test now checks composer submission before control-byte input.
The line-oriented mock otherwise includes those control bytes in its next prompt.

Physical iPhone Safari and Android Chrome testing remains unavailable. Automated
visual-viewport resizing is not evidence of native software-keyboard behavior.
The old final worktree and screenshot bytes could not be read without risking a
source runtime start. Recovery therefore preserves available source snapshots
and explicitly does not claim byte-for-byte restoration of that worktree.
