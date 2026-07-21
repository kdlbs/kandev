---
id: "11-secure-azure-clone"
title: "Secure Azure repository materialization"
status: done
wave: 6
depends_on: ["10-remote-repository-contracts"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 11: Secure Azure Repository Materialization

## Acceptance

- Kandev resolves the active workspace's Azure PAT only inside backend repository materialization.
- Git receives credentials through an ephemeral mechanism; command arguments, logs, stored URLs, task metadata, and agent environments remain credential-free.
- Authentication state is removed after success, failure, timeout, and cancellation.
- GitHub/GitLab clone behavior and separately configured push credentials remain unchanged.

## Verification

- Go tests with a fake Git executable covering credentials, cleanup, cancellation, redaction, and provider isolation.
- Existing repoclone, orchestrator, and task-service suites.
- Security-focused review of every new secret-bearing value.

## Files Likely Touched

- `apps/backend/internal/repoclone/`
- `apps/backend/internal/backendapp/` repository resolver wiring.
- `apps/backend/internal/azuredevops/` credential resolver.
- Repository materialization interfaces and tests.

## Output Contract

Report the credential transport and cleanup guarantees, RED/GREEN commands, security review, and update this task plus its plan checkbox.
