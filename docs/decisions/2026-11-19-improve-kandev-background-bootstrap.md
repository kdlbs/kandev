# ADR-2026-11-19-improve-kandev-background-bootstrap: Run Improve Kandev preparation outside the HTTP request

**Status:** proposed
**Date:** 2026-11-19
**Area:** backend, frontend

## Context

The first bootstrap waits for a repository clone before it sends its reply.
The server has 30 seconds to write that reply by default. In the reported
case the work took about 33 seconds. The proxy returned 502 after the
backend saved the repository and workflows. [Issue #5327](https://github.com/kdlbs/kandev/issues/5327)

## Decision

Keep the setup in the same place in the user flow. Let the bundled dialog
request a background attempt and poll it with short HTTP requests. Return the
existing bootstrap payload when it is ready. A small map in the backend
holds the result for the initiating user. After a backend restart, the
dialog can start another attempt on the existing workspace resources.

## Alternatives considered

- Raise the write deadline for this route. The reverse proxy still has its
  own deadline.
- Defer the clone to task launch. **Create only** can run a passthrough
  launch inside its request, so the same delay can reach a different action.
- Use the global maintenance job tracker. It broadcasts job results. Bootstrap
  results contain data for a specific workspace and diagnostic directory.

See the [bootstrap lifecycle](../specs/workspaces/system-design/improve-kandev-bootstrap.md).
