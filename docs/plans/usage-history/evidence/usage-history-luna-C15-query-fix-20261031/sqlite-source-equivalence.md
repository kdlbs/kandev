SQLite source frame

C12 measured all 32 SQLite scenarios with ten samples each on ee5b9aac81959fa7ee669007cb05f9f6c3a71ae2. It also passed the recent-history comparison.

The integrated query changes select their new SQL only when the driver is PostgreSQL. The SQLite month-edge, currency-safety, fact, coarse, lifetime and undated branches retain their prior relational SQL and bound arguments. The shared source-selection helper preserves the prior Boolean expression. New subject predicates and materialized applied-build sets run only in the PostgreSQL branch. SQLite parity and boundary regressions passed after these changes.

No analytics schema, normalization, canonical selection, projection, amount conversion, query deadline, admission path or benchmark file changed. The chat-state and E2E fixture changes do not enter the backend benchmark. The C12 SQLite measurements remain evidence for the unchanged SQLite query path; they are not represented as a new benchmark run on the later commit. PostgreSQL requires its own full normal rerun.
