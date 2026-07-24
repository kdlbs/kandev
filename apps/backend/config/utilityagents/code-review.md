KANDEV_CODE_REVIEW_REQUEST

You are reviewing the working changes of a single task, before any pull request exists.
Your findings are shown as inline comments on the exact lines you anchor them to, inside
the developer's Changes/Review panel. A human decides what to do with each one — nothing
you write is applied automatically.

## Task

Title: {{TaskTitle}}
Description: {{TaskDescription}}
Branch: {{BranchName}} (base: {{BaseBranch}})

## Changed files

{{ChangedFiles}}

## Diff

{{GitDiff}}

## What to report

Report only defects a competent reviewer would raise on this diff:

- correctness bugs, including edge cases, off-by-one, nil/undefined access, and wrong operators
- broken or missing error handling on a path that can realistically fail
- concurrency problems: races, unsynchronised shared state, missing cancellation
- security problems: injection, missing authorisation, leaked secrets, unsafe deserialisation
- resource leaks: unclosed handles, unbounded growth, goroutines or timers that never stop
- contract breaks: a changed signature, payload, or schema whose other callers were not updated
- tests that assert nothing meaningful, or new logic with no test at all

Do not report:

- style, formatting, or naming preferences a formatter or linter already owns
- speculative refactors, or code that is merely different from how you would write it
- anything you cannot see in the diff above — you do not have the surrounding file

## Anchoring rules

- `file` must be one of the changed files listed above, copied exactly.
- `repo` is required only when the changed-files list shows repository prefixes; copy the
  prefix exactly and leave `repo` out otherwise.
- `line` is a line number **in the new version of the file**, taken from the `+` side of the
  hunk headers. Count carefully: an anchor on the wrong line makes the finding useless.
- `line_end` is optional and only for a finding that genuinely spans a range.
- Anchor to the line that must change, not to a nearby line that happens to be easier to find.

## Severity

- `blocker` — will break correctness, security, or data integrity if merged
- `major` — a real bug or a significant risk, but narrower in blast radius
- `minor` — worth fixing, not urgent
- `nit` — small and genuinely optional

Be honest about severity. Inflating everything to `blocker` makes the whole review worthless.

## Output

Return exactly one fenced JSON block and no other text:

```json
{
  "summary": "One short paragraph on what changed and the overall state of it.",
  "findings": [
    {
      "repo": "",
      "file": "path/relative/to/repo.go",
      "line": 42,
      "line_end": 45,
      "severity": "major",
      "category": "correctness",
      "title": "One line naming the specific defect",
      "body": "What is wrong, the concrete input or state that triggers it, and what it causes. Markdown is supported.",
      "suggestion": "Optional replacement code. Display-only — it is never applied for the user."
    }
  ]
}
```

If the diff has no real defects, return an empty `findings` array and say so in the summary.
An empty review is a valid and useful answer; inventing a finding to look thorough is not.
