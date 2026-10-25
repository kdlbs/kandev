import { describe, expect, it } from "vitest";
import type { Repository, Task } from "@/lib/types/http";
import { buildArchivedValue } from "./task-page-content-helpers";
import { buildArchivedSidebarItem } from "./task-session-sidebar-archived-item";

describe("buildArchivedSidebarItem", () => {
  it("resolves automatic colors from the archived task facts", () => {
    const task = {
      id: "task-1",
      workspace_id: "workspace-1",
      workflow_id: "workflow-1",
      workflow_step_id: "step-1",
      position: 0,
      title: "Archived task",
      description: "",
      state: "TODO",
      priority: "medium",
      archived_at: "2026-08-01T00:00:00Z",
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-08-01T00:00:00Z",
      repositories: [{ id: "link-1", repository_id: "repo-1", base_branch: "main", position: 0 }],
    } as unknown as Task;
    const repository = {
      id: "repo-1",
      workspace_id: "workspace-1",
      name: "web",
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      local_path: "/work/web",
    } as unknown as Repository;
    const value = buildArchivedValue(task, repository);

    const item = buildArchivedSidebarItem(value, {
      repositorySlugById: new Map([["repo-1", "web"]]),
      titleById: new Map(),
      workflowNameById: new Map([["workflow-1", "Delivery"]]),
      stepTitleById: new Map([["step-1", "Todo"]]),
      repositoriesById: new Map([["repo-1", repository]]),
      stepColorById: new Map([["step-1", "blue"]]),
      automaticColorSettings: {
        enabled: true,
        rules: [
          {
            id: "todo-rule",
            enabled: true,
            condition: { dimension: "task_state", value: "TODO", label: "Todo" },
            output: { kind: "fixed", color: "cyan" },
          },
        ],
      },
    });

    expect(item.isArchived).toBe(true);
    expect(item.workflowId).toBe("workflow-1");
    expect(item.state).toBe("TODO");
    expect(item.automaticColor).toEqual({ token: "cyan", className: "bg-cyan-500" });
  });
});
