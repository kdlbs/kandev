import { describe, expect, it } from "vitest";
import type { Repository } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { taskColorProjection } from "./task-color-projection";

const TASK = {
  id: "task-1",
  workspaceId: "workspace-1",
  workflowId: "workflow-1",
  workflowStepId: "step-1",
  title: "Task",
  position: 0,
  repositories: [{ id: "link-1", repository_id: "repo-1", base_branch: "main", position: 0 }],
} as KanbanState["tasks"][number];

const REPOSITORY = {
  id: "repo-1",
  workspace_id: "workspace-1",
  name: "kandev",
  source_type: "github",
  local_path: "",
  provider: "github",
  provider_repo_id: "kdlbs/kandev",
  provider_host: "github.com",
  provider_scope: "kdlbs",
} as Repository;

describe("taskColorProjection", () => {
  it("joins saved repository identity and workflow step color before resolving", () => {
    const result = taskColorProjection(TASK, {
      settings: {
        enabled: true,
        rules: [
          {
            id: "repository-rule",
            enabled: true,
            condition: {
              dimension: "repository",
              value: {
                kind: "provider",
                provider_id: "github",
                host: "github.com",
                scope: "kdlbs",
                provider_repository_id: "kdlbs/kandev",
              },
              label: "kdlbs/kandev",
            },
            output: { kind: "fixed", color: "cyan" },
          },
        ],
      },
      repositoriesById: new Map([[REPOSITORY.id, REPOSITORY]]),
      stepColorById: new Map([["step-1", "emerald"]]),
    });

    expect(result?.color).toEqual({ token: "cyan", className: "bg-cyan-500" });
    expect(result?.source.ruleId).toBe("repository-rule");
  });

  it("uses the workflow step color when the winning output follows the step", () => {
    const result = taskColorProjection(TASK, {
      settings: {
        enabled: true,
        rules: [
          {
            id: "step-rule",
            enabled: true,
            condition: {
              dimension: "workflow_step",
              value: { workspace_id: "workspace-1", step_id: "step-1" },
              label: "Review",
            },
            output: { kind: "workflow_step" },
          },
        ],
      },
      repositoriesById: new Map(),
      stepColorById: new Map([["step-1", "#abc"]]),
    });

    expect(result?.color).toEqual({ token: "custom", style: { backgroundColor: "#abc" } });
  });
});
