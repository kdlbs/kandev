# Review notes

## Fixed during review

- `apps/backend/internal/testutil/envscan.go:281` — the round-5 conflict check
  covered only constants declared with two *different string literals*. A
  build-tagged pair that declares one name twice with only **one** side written
  as a literal (the other referring to another constant, concatenating, or
  repeating the previous expression list) produced no conflict, so the literal
  file's value was lent to the read in the file the scanner cannot read. Where
  that borrowed value is covered, the read is classified against the wrong
  variable exactly as silently as the case round 5 fixed. Any name the scanner
  cannot read a string literal from is now recorded unresolvable, so no sibling
  declaration can stand in for it. Tracked separately from `conflicts` so the
  build-tag message is not shown for a case with no second literal.
  (commit `187f0582f`)

  Proven against the real `process` package before and after: a
  `//go:build !windows` file declaring `qaEnv = qaOtherEnv` (where `qaOtherEnv`
  is the uncovered `"KANDEV_QA_UNCOVERED"`) and reading `os.Getenv(qaEnv)`,
  paired with a `//go:build windows` file declaring `qaEnv = "SHELL"`, left the
  guard reporting `ok`; it now fails. Neutering `recordUnresolvable` makes the
  two new tests the only failures.

## Follow-up tasks created (out of scope for this PR)

None.

## Action required by author

None. Two round-5 design assumptions were put to review and are **confirmed**:

- A constant whose value depends on which build-tagged file you believe is
  reported **unresolvable** rather than resolved to one platform's value. This
  is the same shape as every earlier round's fix — it only adds a failure mode,
  never removes one — and the message names the constant and the build-tag
  cause. The review fix above extends the same principle to the declaration
  forms the scanner cannot read.
- The bulk-read (`os.Environ`, `filterGitEnv(g.environmentValues())`) and
  non-`os`-API (`syscall.Getenv`) boundaries are **documentation, not defects**.
  An argument-less read carries no name, so a name-based guard structurally
  cannot classify it; both are stated in the guard's doc comment.

Note for whoever updates the task plan: section 10's build-tag limitation now
has this second spelling closed as well, and section 14 gains this review round.
The plan was left untouched by this phase.
