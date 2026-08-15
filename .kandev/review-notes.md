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

- `apps/backend/internal/testutil/envscan_test.go:484` — QA round 6's boundary
  test pinned only *half* of what its own comment claimed. It asserted that an
  unreadable const spec does not poison its block, but every declaration in its
  snippet was one name per spec, so a hoist of `recordUnresolvable` from name
  level to **spec** level was inert and the test passed unchanged. Verified by
  mutation in review round 7: with the spec-level hoist applied, the whole
  `internal/testutil` suite passed and both live package guards reported `ok` —
  the same blind spot the test was written to close, one level down. The
  snippet now also declares `bazEnv, unit = "BAZ", timeout`, a multi-name spec
  whose second name is unreadable, so the covered read of `bazEnv` fails under
  a spec-level hoist. Both hoists are now killed by this one test, and each is
  the only failure under its mutant. Test-only, no guard behaviour change.

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

The task plan was brought up to date in QA round 6 (sections 6, 8, 10, 13 and
14), so section 10's build-tag limitation records both spellings and section 14
records every round.
