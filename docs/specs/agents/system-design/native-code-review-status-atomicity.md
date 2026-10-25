---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
---

# Native review finding status atomicity

The task review repository is the owner of persisted finding state. A status
transition updates `status` and `resolved_at` in one database statement and
returns the row produced by that statement. The service validates the requested
status before it calls the repository, then publishes the existing finding
updated event with the returned row.

Concurrent writers can therefore observe different final rows without creating
a timestamp that describes an earlier read. SQLite tests provide deterministic
coverage. A PostgreSQL test with two database connections checks the same
transaction boundary when a PostgreSQL environment is available.
