---
title: Optional clarification answer provenance
status: done
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
system_design:
  - ../../specs/integrations/system-design/external-question-answering-01.md
created: 2026-10-23
---

# Optional clarification answer provenance

Carry an optional caller-supplied answer channel and reference through the
existing REST and MCP resolution paths. Persist the winner's marker in the
same atomic bundle claim and return it on durable replay. Keep unmarked
historical answers and the existing authorization, normalization and delivery
rules unchanged. Provenance never grants authority.

The implementation is scoped in [Task 01](task-01-answer-provenance.md).
This delivery adds no schema, Office workflow, operating option, permanent
test, or new validation procedure.

The focused checks passed through REST/native MCP, the real SQLite claim and
winner readback, optional external-tool metadata, zero-value serialization
and existing retry/delivery behavior. The normal commit hooks must pass
before publication.
