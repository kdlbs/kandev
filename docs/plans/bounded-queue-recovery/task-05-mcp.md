---
id: "05-mcp"
title: "Bounded queue recovery: mcp"
status: done
wave: 5
depends_on: ["04-authority"]
plan: plan.md
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-QUEUE-RECOVERY-001.8
  - AC-TASKS-QUEUE-RECOVERY-001.9
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
---

# Work packet 5: mcp

## Scope
Implementation Step 5 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 5's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
cd apps/backend && go test -count=1 ./internal/mcp/server ./internal/mcp/handlers ./internal/gateway/websocket -run 'Test(QueueRecovery|CoreToolRiskAnnotations|RegisterTools_LoggedCountMatchesRegisteredTools|MCPAction)'
```

## Risks and results
Authenticated MCP inventory/body forwarding and profile/raw-dispatch boundaries passed focused tests at `56a5f0e4`; distinct Review and QA passed that exact candidate with task-owned PostgreSQL 16.15 parity. PR corrections require delta verification. Census registration remains with task `ca015838`; release acceptance is pending.
