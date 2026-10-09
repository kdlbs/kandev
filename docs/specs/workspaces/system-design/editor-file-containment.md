---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-EDITOR-CONTAINMENT-001
---

# Editor File Containment System Design

## Boundary and ownership

The editors service is a consumer of the Workspaces lexical target contract.
Its resolved worktree supplies the root for every file-level editor kind.
Tasks owns session/worktree association and repository fallback. The
[embedded-editor availability decision](../../../decisions/2026-07-30-embedded-editor-executor-capabilities.md)
continues to control eligibility independently.

This design documents the existing lexical boundary precisely. It does not
replace it with physical containment, change filesystem permissions, or
establish a new architectural boundary requiring an ADR.

## Requirement mapping

| Criteria | Design sections |
| --- | --- |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.1`, `AC-WORKSPACES-EDITOR-CONTAINMENT-001.2` | Lexical admission, Integration compatibility, Verification |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.3` | Lexical admission, Failure contract, Verification |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.4` | Existing flow, Failure contract, Verification |

## Existing flow

`Service.OpenEditor` in `apps/backend/internal/editors/service/service.go`
reads the session and settings, resolves the selected editor, and enforces
embedded-editor capability. Its empty embedded file target returns
`internal://vscode` before worktree resolution, including for repository-less
sessions. Other requests use `resolveSessionPath`, then `resolveFilePath`, then
`dispatchEditorKind`.

`resolveSessionPath` keeps explicit worktree/association selection, first-worktree
fallback, and repository local-path fallback. None of those branches changes.

## Lexical admission

Keep the current empty-root and empty-file handling, `filepath.Clean`,
`filepath.Join`, and `filepath.Rel` sequence in `resolveFilePath`. A `Rel` error
still returns `ErrEditorConfigInvalid`. Reject a relative result equal to `..`
or beginning with `..` followed by the native path separator. Do not reject a
name solely because its first two characters are dots.

The fixed seam is this existing guard only. Native `filepath` normalization
already reduces complete internal `.` and `..` components before admission.
Do not add a shared path framework, an allowlist of filenames, filesystem
existence checks, `EvalSymlinks`, or `filepath.IsLocal` as a replacement policy.
Existing absolute-input joining and cross-volume behavior are outside this
repair and must not be redesigned incidentally.

## Integration compatibility

| Integration | Input after admission | Planned evidence | Limits |
| --- | --- | --- | --- |
| Embedded VS Code | Existing worktree root and absolute target, then relative sentinel goto | Real `OpenEditor`, actual fixture bytes, parsed target/line/column | URL acceptance only; no code-server or panel launch |
| Hosted URL | Absolute path in existing `file` query | Existing `TestBuildHostedURL` and shared resolver tests | No remote-service request |
| Remote SSH URL | Absolute path and existing line/column suffix | Existing `TestBuildRemoteSSHURL` and shared resolver tests | No URI handler or SSH launch |
| Built-in/custom command | Existing absolute target and working-directory contract | Shared resolver coverage; existing argument/placeholder tests | No native process launch or CLI interpretation claim |

Unknown or unavailable editors retain their existing error path; target
admission cannot grant editor eligibility. Shared guard coverage does not imply
native integration testing for every editor or executor.

## Failure contract

`Controller.OpenSessionEditor` transfers `dto.OpenEditorRequest` fields directly
to `Service.OpenEditor` and returns the existing optional `url` response.
`handlers.RegisterRoutes` registers
`POST /api/v1/task-sessions/:id/open-editor`.
`serviceErrorStatus` retains 400 for invalid configuration, 404 for missing
workspace/editor, and 409 for unavailable editor. No handler, error wording,
logging, DTO, schema, frontend caller, or transport change is planned.

No persistence, migration, retry, metric, or new routine log is needed.

## Native platform qualification

Tests construct roots and normal fixture paths with `t.TempDir` and
`filepath.Join`/`filepath.FromSlash`; native separators are tested on the host
actually executing the Go test. A Linux run proves Linux semantics only. On
Windows, cover both accepted separator forms where `filepath` treats them as
separators; on POSIX, a backslash remains a filename character and must not be
invented as traversal. Do not claim Windows execution from a Linux string
fixture or cross-compilation. Physical symlink escapes remain a residual of
the existing lexical policy.

## Verification

Add `service/editor_file_containment_test.go` with
`TestOpenEditor_ContainedDotPaths` and
`TestResolveFilePath_ContainmentControls`. Reuse the existing minimal
`openEditorRepository`, `openEditorTaskRepository`, and `openEditorUserSettings`
fixtures; use a Local Docker capability value to permit sentinel generation
without starting Docker. Run the actual production service against existing
temporary files with distinct sentinel bytes.

The real-service regression must fail before the fix for `..notes.go` and
`..notes/inside.go`, while an ordinary file and true escape control pass. After
the correction, assert the full decoded goto identity and line/column, exact
resolved path, exact read-back bytes, and unchanged fixture bytes. Target
assertions must tolerate the separately owned URL-encoding repair by decoding
the existing query rather than comparing incidental raw percent encoding.

Cover `..`, `../outside.go`, nested escapes, ordinary files, `.hidden.go`,
`...notes.go`, nested dot-prefixed segments, repeated native separators, `.`
normalization, internal parent normalization, empty root, and empty file.
Use explicit expected outcomes; never copy the production predicate into the
test or assert source text. Rejected service cases must return the exact
error category and empty target.

Existing `handlers/TestServiceErrorStatus` protects the distinct HTTP mapping.
The current handler package has no registered-route/service fixture; do not
build a new fixture framework for this seam. If a cheap existing registered
fixture is discovered during implementation, extend it with 200 exact-target
and 400 empty-target controls without bypassing the real service. Otherwise
report that registered routing was not exercised. Service-to-sentinel proof
is the end-to-end operation boundary for this package, not native-app proof.

## Presentation and public documentation

No rendered layout, touch behavior, scrolling, navigation, focus, or copy
changes. The shared backend input normalization satisfies the internal
state/data mobile exception; no browser, phone E2E, or preview is required.
`docs/public/developer-tools.md` already describes file-level editor opening
and its worktree target. Restoring contained filenames changes no documented
option, terminology, workflow, or API shape, so public docs need no edit.
