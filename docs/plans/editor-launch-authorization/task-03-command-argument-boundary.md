---
id: "03-command-argument-boundary"
title: "Placeholder values stay inside their argument"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EDITOR-COMMAND-001
acceptance_criteria:
  - AC-WORKSPACES-EDITOR-COMMAND-001.1
  - AC-WORKSPACES-EDITOR-COMMAND-001.2
  - AC-WORKSPACES-EDITOR-COMMAND-001.3
  - AC-WORKSPACES-EDITOR-COMMAND-001.4
system_design:
  - ../../specs/workspaces/system-design/editor-command-arguments.md
---

# Task 03: Placeholder values stay inside their argument

## Summary

`launchCommand` splits the administrator's template into arguments first,
then substitutes raw placeholder values into each argument.

## In scope

- `apps/backend/internal/editors/service/service.go`:
  - Extract `buildCommandArgs(template, worktreePath, absPath string, line, column int) ([]string, error)`:
    `securityutil.SplitShellCommand(template)`, return `ErrEditorConfigInvalid`
    for an empty result and the existing wrapped error for a tokenizer error,
    then replace `{cwd}`, `{file}`, `{rel}`, `{line}`, `{column}` in each token
    with raw values. `{rel}` keeps its current `filepath.Rel` fallback.
  - `launchCommand` uses `buildCommandArgs` and the process-start seam from
    task 02 (or adds it if task 03 lands first).
  - Remove `expandCommandPlaceholders` and its `ShellEscape` use.
- `apps/backend/internal/editors/service/service_test.go`: replace
  `TestExpandCommandPlaceholders` with `TestBuildCommandArgs`.

## Out of scope

Authorization (tasks 01, 02), target containment, built-in editor arguments
(`buildLocalArgs`), and how launched programs parse their arguments.

## Acceptance

1. `TestBuildCommandArgs` table, each case asserting the exact argument
   vector: templates `code {file}`, `code "{file}"`, `code '{file}'`,
   `code --goto {file}:{line}:{column}`, `code --goto="{file}"`; file names
   containing a space, `"`, `'`, `\`, `$(id)`, a backtick, `;`, `|`, `&`, and
   `a" --evil "b`. The argument count always equals the template token count.
   The `code "{file}"` case with `a" --evil "b` is the regression test that
   fails before the change.
2. Unquoted ordinary paths produce the same vector as before the change
   (compatibility witness).
3. An unclosed quote in the template and an empty template return
   `ErrEditorConfigInvalid`, and no process starts.

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/common/securityutil/... ./internal/editors/service/...)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
```

Mutation: reintroduce substitution before splitting; the regression case must
fail.

## Likely files

- `apps/backend/internal/editors/service/service.go`
- `apps/backend/internal/editors/service/service_test.go`

## Results

Command templates are tokenized before placeholder substitution. Tests cover
the requested templates and hostile file names, preserve the argument count,
verify relative and ordinary paths, and reject empty or malformed templates
without starting a process. The tokenizer checks quote state after processing
the complete input, including an unclosed quote ending in a multibyte rune or
an escaped final quote.

Verification passed:

- `go test -count=1 ./internal/common/securityutil/... ./internal/editors/service/...`
- `git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed` (0 issues)
