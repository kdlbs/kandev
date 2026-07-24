import { describe, expect, it } from "vitest";
import { resolveComposerWorkspaceId } from "./composer-workspace";

const workflows = [{ id: "workflow-1", workspaceId: "workspace-1" }];

describe("resolveComposerWorkspaceId", () => {
  it("uses the workspace persisted with a Quick Chat session", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: "quick-session",
        taskId: null,
        quickChatSessions: [{ sessionId: "quick-session", workspaceId: "quick-workspace" }],
        activeWorkflowId: null,
        activeTasks: [],
        snapshots: [],
        workflows,
      }),
    ).toBe("quick-workspace");
  });

  it("maps a task through its loaded workflow", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: "task-session",
        taskId: "task-1",
        quickChatSessions: [],
        activeWorkflowId: "workflow-1",
        activeTasks: [{ id: "task-1" }],
        snapshots: [],
        workflows,
      }),
    ).toBe("workspace-1");
  });

  it("fails closed when the task cannot be mapped to a workflow", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: "unknown-session",
        taskId: "unknown-task",
        quickChatSessions: [],
        activeWorkflowId: "workflow-1",
        activeTasks: [{ id: "another-task" }],
        snapshots: [],
        workflows,
      }),
    ).toBeNull();
  });
});
