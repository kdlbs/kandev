---
status: draft
system: agents
created: 2026-10-12
owners:
  - kandev
---

# Native Code Review Reviewer Exclusion Requirements

## Overview

The single-model workflow lets one agent profile own a task from planning
through merge while a native code-review pass supplies independent
verification. That value depends on the reviewer actually being a **different**
model than the one that wrote the change; a review that silently runs on the
writer's own model adds no independent signal. This document defines the
observable contract for excluding the writer's own execution from reviewer
selection.

## Terminology

- **Origin execution:** the concrete agent execution that produced the changed
  files under review, identified by its logical agent profile, its concrete
  execution profile, and its effective agent and model.
- **Distinct reviewer:** a reviewer candidate whose concrete profile, or whose
  effective agent and model, differ from the origin execution.

## Requirements

### REQ-AGENTS-NATIVE-CODE-REVIEW-002: Reviewer distinct from the writer

**Intent:** A workflow-step review must never silently select the writer's own
model as the reviewer, so the review stays independent, and an operator can see
when no distinct reviewer is available.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-CODE-REVIEW-002.1:** When a `run_code_review` step-entry action starts a review, the system shall supply the origin session's concrete execution identity to reviewer selection.
- **AC-AGENTS-NATIVE-CODE-REVIEW-002.2:** When selecting a reviewer, the system shall exclude a candidate whose concrete profile is the origin's, or whose effective agent and model are the origin's, and shall preserve the order and eligibility of every remaining candidate.
- **AC-AGENTS-NATIVE-CODE-REVIEW-002.3:** When every configured reviewer candidate matches the origin execution, the system shall fail the review with `review_agent_unavailable` and write no findings, and the task shall still enter the step.
- **AC-AGENTS-NATIVE-CODE-REVIEW-002.4:** When a `run_code_review` step-entry action cannot read the origin session's identity, the system shall fail the review with `review_agent_unavailable` and write no findings rather than reviewing with an unprovable writer model, and the task shall still enter the step.
- **AC-AGENTS-NATIVE-CODE-REVIEW-002.5:** The built-in workflow template format shall accept a `run_code_review` `on_enter` action with an optional string `agent_profile_id`, and shall reject a non-string `agent_profile_id`.

## Out of scope

- Automatic delivery of findings back to the origin session; the writer reads
  them through the existing review surface.
- Ranking or reordering reviewer candidates beyond removing origin matches.
- Office participant/quorum review, which is a separate ownership-transfer path.
- Manual reviewer selection; selection stays on the configured candidate order.
