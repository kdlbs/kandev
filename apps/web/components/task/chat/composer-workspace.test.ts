import { describe, expect, it } from "vitest";
import { resolveComposerWorkspaceId } from "./composer-workspace";

const WORKFLOW_ONE = "workflow-1";
const WORKSPACE_ONE = "workspace-1";
const TASK_ONE = "task-1";
const TASK_SESSION = "task-session";
const workflows = [{ id: WORKFLOW_ONE, workspaceId: WORKSPACE_ONE }];

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
        sessionId: TASK_SESSION,
        taskId: "task-1",
        quickChatSessions: [],
        activeWorkflowId: WORKFLOW_ONE,
        activeTasks: [{ id: TASK_ONE }],
        snapshots: [],
        workflows,
      }),
    ).toBe(WORKSPACE_ONE);
  });

  it("uses the workspace of an exact Office task when no Kanban collection is loaded", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: TASK_SESSION,
        taskId: "office-task",
        quickChatSessions: [],
        activeWorkflowId: null,
        activeTasks: [],
        snapshots: [],
        workflows: [],
        officeTasks: [{ id: "office-task", workspaceId: "office-workspace" }],
      }),
    ).toBe("office-workspace");
  });

  it("fails closed when task-bound collections disagree about the workspace", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: TASK_SESSION,
        taskId: "task-1",
        quickChatSessions: [],
        activeWorkflowId: WORKFLOW_ONE,
        activeTasks: [{ id: TASK_ONE }],
        snapshots: [],
        workflows,
        officeTasks: [{ id: TASK_ONE, workspaceId: "office-workspace" }],
      }),
    ).toBeNull();
  });

  it("fails closed when the task cannot be mapped to a workflow", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: "unknown-session",
        taskId: "unknown-task",
        quickChatSessions: [],
        activeWorkflowId: WORKFLOW_ONE,
        activeTasks: [{ id: "another-task" }],
        snapshots: [],
        workflows,
      }),
    ).toBeNull();
  });

  it("uses the task's matching workflow snapshot and ignores unrelated workspace rows", () => {
    expect(
      resolveComposerWorkspaceId({
        sessionId: TASK_SESSION,
        taskId: "snapshot-task",
        quickChatSessions: [],
        activeWorkflowId: "workflow-1",
        activeTasks: [{ id: "unrelated-task" }],
        snapshots: [
          { workflowId: "workflow-2", tasks: [{ id: "snapshot-task" }] },
          { workflowId: "workflow-3", tasks: [{ id: "another-task" }] },
        ],
        workflows: [
          ...workflows,
          { id: "workflow-2", workspaceId: "task-workspace" },
          { id: "workflow-3", workspaceId: "unrelated-workspace" },
        ],
      }),
    ).toBe("task-workspace");
  });
});
