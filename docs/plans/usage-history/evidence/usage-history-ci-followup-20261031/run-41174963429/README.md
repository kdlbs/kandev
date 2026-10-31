# CI audit and pending corrections

Core source: `1ce8d0bfdab088f899cf0ede7a5a911d097e0cee`.

Run `41174963429`, attempt one, passed all 42 workflow jobs. The complete raw audit still found three flaky verdicts. The zero-flake gate therefore failed.

The audit reconciled 4,599 test identities and 4,602 attempts across 118 reports. The parent independently verified 69 archive hashes and 496 extracted members. Three tests failed on their first attempt and passed on retry. QuickChat, Office and Plan passed on their first attempt.

B26, B27 and B28 prepared reviewed corrections in three E2E specs. They preserve the original UI assertions. Their baseline and browser checks remain pending while C10 runs the full database benchmarks. This packet does not claim that these corrections pass.

The receipts describe the source evidence and its limits. Failed attempts have no trace; passing retry traces are not failure evidence. The profile error-context duplication remains documented.

Raw archives remain in the workflow artifacts. This package retains the complete attempt ledger, audit, source diffs, implementation handoffs and parent reviews.

B29 later completed three unmodified baseline and 29 changed-case executions with zero retries. See [the browser verification](B29-verification/README.md). A fresh zero-flake CI audit remains required.
