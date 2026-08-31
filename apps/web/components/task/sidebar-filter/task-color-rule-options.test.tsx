import { describe, expect, it } from "vitest";
import { buildTaskColorRuleOptions, taskColorRuleOptionKey } from "./task-color-rule-options";

describe("automatic color rule options", () => {
  it("uses workspace-aware values for workflows and steps", () => {
    const options = buildTaskColorRuleOptions(
      {
        workflows: [{ id: "workflow-1", workspaceId: "workspace-1", name: "Delivery" }],
        snapshots: {
          "workflow-1": {
            workflowId: "workflow-1",
            workflowName: "Delivery",
            steps: [{ id: "step-1", title: "Review", color: "bg-amber-500", position: 0 }],
            tasks: [],
          },
        },
        executorProfiles: [],
      },
      (key) => key,
    );

    expect(options.workflow[0]?.value).toEqual({
      workspace_id: "workspace-1",
      workflow_id: "workflow-1",
    });
    expect(options.workflow_step[0]?.value).toEqual({
      workspace_id: "workspace-1",
      step_id: "step-1",
    });
  });

  it("provides translated scalar options and stable keys", () => {
    const options = buildTaskColorRuleOptions(
      { workflows: [], snapshots: {}, executorProfiles: [], activeWorkspaceId: null },
      (key) => `translated:${key}`,
    );
    expect(options.task_state.find((option) => option.value === "BLOCKED")?.label).toBe(
      "translated:common:taskStateBlocked",
    );
    expect(options.priority[0]?.key).toBe(taskColorRuleOptionKey("critical"));
    expect(options.origin.some((option) => option.value === "kanban")).toBe(true);
  });
});
