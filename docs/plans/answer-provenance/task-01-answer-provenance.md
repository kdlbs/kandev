---
id: answer-provenance-01
title: Preserve optional clarification answer provenance
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
acceptance_criteria:
  - AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.1
  - AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.3
  - AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.9
system_design:
  - ../../specs/integrations/system-design/external-question-answering-01.md
---

# Preserve optional clarification answer provenance

## Scope

- Add optional `answered_by.channel` and `answered_by.ref` to the existing
  clarification response, REST outcome and native/external MCP input.
- Pass a typed bundle-wide marker through the existing claim map, store it
  on the winning message rows and reconstruct that winner on replay.
- Omit zero markers from normal and durable JSON. Remove the marker on
  definite-delivery rollback; preserve accepted or uncertain outcomes.
- Preserve existing answer validation, ordering, authorization and retry
  behavior. No frontend, Office, inbox, schema or operating-policy changes.

## Verification

Reused the source's temporary provenance fixtures outside automatic
collection together with existing clarification and question-handler
checks. Actual REST/native MCP entry points backed by SQLite persisted and
replayed the winner's marker; a losing request could not overwrite it.
Optional external-tool forwarding, zero-value JSON omission, durable
restore and existing accepted/rejected retry behavior passed. No permanent
tests were added.

The selected command used `go test -tags fts5 -trimpath -p 2 -count=1` with
the temporary overlay and the affected clarification, MCP server, MCP
handler and SQLite repository packages. Package results were PASS (0.167s,
0.122s, 2.435s and 2.709s respectively). The existing SQL guard passed.
The existing documentation-coverage validator reported `covered` with no
errors. Normal commit hooks must pass before publication.

PostgreSQL and live third-party messaging are not exercised by this source
delivery. The existing database dialect and authorization paths are retained.
