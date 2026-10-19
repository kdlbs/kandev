---
created: 2026-10-14
status: completed
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
legacy_specs: []
---

# Literal executor placeholder expansion

## Outcome and requirement reconciliation

Restore literal repository branch data during executor command construction.
The executor system owns the failed script-boundary contract.
Reuse the active [repository branch requirements](../../specs/executors/requirements/repository-branch-resolution.md),
specifically AC-EXECUTORS-REPOSITORY-BRANCH-001.2, .4, and .5.
The requirements remain unchanged; the owning design now specifies expansion
from the original template and opaque inserted values.

## Evidence and root cause

`Resolver.Resolve` iterates an unordered merged map and runs `strings.ReplaceAll`
against the evolving result. A replacement value can therefore become input to
a later replacement. With the real `RepositoryProvider` and `WorkspaceProvider`,
the valid plain branch `topic/{{workspace.path}}` can become a branch containing
the workspace path. Existing provider shell quoting cannot prevent this textual
mutation. The supplied diagnosis reports a native Git checkout failure for the
mutated command and success for the independent literal-command control.
Implementation must establish its own RED with new repository-owned tests.

## Scope and approach

One sequential work order adds independent regression tests, observes the
causal failure, changes only production `Resolver.Resolve`, and verifies the
owning package. Preserve `mergedVars`, provider registration order, static
registration behavior, literal unknown tokens, and existing provider quoting.
Use a single `strings.NewReplacer` operation over the original script rather
than repeated replacement over inserted text.

Production ownership is limited to
`apps/backend/internal/scriptengine/resolver.go`. New tests live in
`resolver_test.go` and `resolver_git_test.go` in that package. The design and
this delivery package own documentation changes.
No branch normalization, provider changes, API/storage migration, executor
redesign, lifecycle validation change, security grant, or recursive interpolation
is included. Desktop and phone composition are unchanged; no frontend test,
typecheck, build, browser test, or screenshot is needed. A fresh worktree may
need one frozen workspace install after implementation solely to supply the
normal commit-hook dependencies, as described in the work order.

## Compatibility evidence

All seven production resolver callers assemble their templates before one
resolution call. Managed checkout fragments appended before that call remain
part of the original template. Provider-returned setup/install/auth/controller
fragments remain opaque inserted shell text, with their returned bytes intact.
Existing registry descriptions, fragment tests, executor docs, and the owning
branch design contain no nested-expansion promise. A custom fragment that relied
on accidental map-order expansion will retain embedded tokens instead.

| Boundary | Intended result | Evidence and limit |
| --- | --- | --- |
| Repository and workspace providers | Quoted branch/path bytes remain intact | Real providers plus local native Git and exact render assertions |
| Static and ordered dynamic providers | Existing collision precedence | Resolver unit controls, including later provider registration |
| Setup and install fragments | Insert returned shell text verbatim | Exact byte controls with known/unknown tokens and shell syntax |
| Docker, Sprites, Kubernetes, host preparation | Shared single-pass resolution | Source audit; no live transport claim |
| SSH preparation/cleanup | Shared resolution followed by existing caller validation | `resolveSSHScript` still rejects any remaining `{{`; literal-token launch support is outside this repair |

## Tests and end-to-end evidence

The work order defines the exact new test names and commands.
`TestResolverLiteralBranchCheckout` covers AC-001.2 and .4 using actual providers,
shell execution, a disposable Git remote, current fetch-style and saved
clone-style command construction, and distinct branch/HEAD/sentinel assertions.
`TestResolverLiteralBranchMissing` covers AC-001.5 without a default-branch
fallback. `TestResolverOriginalTemplateOnly` checks all 128 renders against
independent literal expected text, sampling map order without retries.
`TestResolverCompatibility` and `TestResolverScriptFragmentsVerbatim` cover
the existing resolution and fragment contracts.
These proposed names describe tests to author during implementation, not tests
already present or results already obtained.

The native Git test is the end-to-end evidence for this backend command boundary.
Existing caller/default/saved-template integration is not required for the
minimum fix because no caller or template changes and all callers use this seam.
Only if implementation uncovers a new concrete caller-specific uncertainty, run the existing
`TestSpritesPrepareScript_OriginBranch`, `TestSpritesPrepareScript_MissingBranch`,
and `TestBranchNameCommandInjection_Regression` selection documented in the
work order. Do not expand to a whole lifecycle, orchestrator, or backend suite.

## Public documentation

The existing [Git operations reference](../../public/git-operations.md#branch-names-in-executor-scripts)
already promises unchanged plain branches, current/saved template behavior,
and missing-branch failure. The [executor explanation](../../public/executors.md#script-behavior-is-runtime-specific)
does not promise nested fragment expansion. This repair restores that published
behavior and does not require a public documentation edit. The internal design
records the exact interpolation semantics and the existing SSH limitation.

## Work orders

- [x] [Task 01: Restore literal template expansion](task-01-literal-expansion.md)

## Verification results

Completed on 2026-10-14. The single bounded pre-fix RED invocation produced
83/128 mutated current-fetch commands and 84/128 mutated saved-clone commands;
both executed commands failed in native Git with exit 128. Independent literal
controls succeeded at the selected branch and HEAD with exact sentinel bytes.
Ordinary/repeated/unknown/empty/precedence compatibility controls passed.
After the single-pass correction, the complete owning race package passed
in 1.403 seconds and canonical affected-package lint reported zero issues.
Catalog, specification lint, diff checks, and actual six-path documentation
coverage passed. The [work order](task-01-literal-expansion.md#results) records
the exact commands and outcomes. The existing caller selection was not needed;
no new caller-specific uncertainty arose.
The two historical linked packages in the design are implemented records;
this repair changes neither their scope nor their prior results.

## Risks

- The pre-fix defect depends on map iteration order. Render sampling is fixed
  at 128; absence of a causal RED is unresolved evidence, not permission to
  retry until failure or start the correction.
- Literal-token SSH values can still be rejected by the existing caller check.
  Native resolver/Git success must not be reported as live SSH or Docker success.
- Custom fragments that relied on incidental nested replacement will retain
  embedded tokens. No supported nested contract was found in the audited sources.
