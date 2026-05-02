import { describe, expect, it } from "vitest";
import type { RecentTaskEntry } from "@/lib/recent-tasks";
import type { Repository, TaskSession } from "@/lib/types/http";
import {
  buildRecentTaskDisplayItems,
  buildRecentTaskEntry,
  getInitialSelectionIndex,
  getNextSelectionIndex,
  getPreviousSelectionIndex,
  getTaskStatusBadge,
  type RecentTaskBuildContext,
} from "./recent-task-switcher-model";

const visitedAt = "2026-05-02T10:00:00Z";
const WORKSPACE_ID = "workspace-1";
const REPOSITORY_ID = "repo-1";
const CURRENT_TASK_ID = "task-current";
const PREVIOUS_TASK_ID = "task-previous";
const WF_MAIN = "wf-1";
const WF_REVIEW = "wf-2";

function recent(taskId: string, title: string): RecentTaskEntry {
  return {
    taskId,
    title,
    visitedAt,
    taskState: "TODO",
    sessionState: null,
    repositoryPath: null,
    workflowId: WF_MAIN,
    workflowName: "Stored Workflow",
    workflowStepTitle: "Stored Step",
    workspaceId: WORKSPACE_ID,
  };
}

function session(taskId: string, state: TaskSession["state"]): TaskSession {
  return {
    id: `${taskId}-session`,
    task_id: taskId,
    state,
    started_at: visitedAt,
    updated_at: visitedAt,
    is_primary: true,
  };
}

function buildContext(): RecentTaskBuildContext {
  const repository: Repository = {
    id: REPOSITORY_ID,
    workspace_id: WORKSPACE_ID,
    name: "Local Repo",
    source_type: "local",
    local_path: "/tmp/local-repo",
    provider: "github",
    provider_repo_id: "repo-gh-id",
    provider_owner: "kdlbs",
    provider_name: "kandev",
    default_branch: "main",
    worktree_branch_prefix: "task",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    created_at: visitedAt,
    updated_at: visitedAt,
  };

  return {
    activeTaskId: CURRENT_TASK_ID,
    kanbanWorkflowId: WF_MAIN,
    kanbanTasks: [
      {
        id: CURRENT_TASK_ID,
        workflowStepId: "step-1",
        title: "Current live title",
        position: 0,
        state: "IN_PROGRESS",
        repositoryId: REPOSITORY_ID,
        primarySessionId: `${CURRENT_TASK_ID}-session`,
        createdAt: visitedAt,
        updatedAt: visitedAt,
      },
    ],
    kanbanSteps: [{ id: "step-1", title: "Working", color: "bg-blue-500", position: 0 }],
    snapshots: {
      [WF_REVIEW]: {
        workflowId: WF_REVIEW,
        workflowName: "Review Flow",
        steps: [{ id: "step-2", title: "Review", color: "bg-green-500", position: 0 }],
        tasks: [
          {
            id: PREVIOUS_TASK_ID,
            workflowStepId: "step-2",
            title: "Previous live title",
            position: 0,
            state: "REVIEW",
            repositoryId: REPOSITORY_ID,
            createdAt: visitedAt,
            updatedAt: visitedAt,
          },
        ],
      },
    },
    workflows: [
      { id: WF_MAIN, workspaceId: WORKSPACE_ID, name: "Main Flow" },
      { id: WF_REVIEW, workspaceId: WORKSPACE_ID, name: "Review Flow" },
    ],
    repositoriesByWorkspace: { [WORKSPACE_ID]: [repository] },
    sessionsByTaskId: { [CURRENT_TASK_ID]: [session(CURRENT_TASK_ID, "RUNNING")] },
    gitStatusByEnvId: {},
    environmentIdBySessionId: {},
  };
}

describe("recent task switcher model", () => {
  it("starts selection on the first non-current task when possible", () => {
    const items = [
      { taskId: CURRENT_TASK_ID },
      { taskId: PREVIOUS_TASK_ID },
      { taskId: "task-older" },
    ];

    expect(getInitialSelectionIndex(items, CURRENT_TASK_ID)).toBe(1);
    expect(getInitialSelectionIndex(items, "task-missing")).toBe(0);
    expect(getInitialSelectionIndex([], CURRENT_TASK_ID)).toBe(-1);
  });

  it("cycles through items and handles empty lists", () => {
    expect(getNextSelectionIndex(0, 3)).toBe(1);
    expect(getNextSelectionIndex(2, 3)).toBe(0);
    expect(getNextSelectionIndex(-1, 3)).toBe(0);
    expect(getNextSelectionIndex(0, 0)).toBe(-1);
  });

  it("cycles backward through items and handles empty lists", () => {
    expect(getPreviousSelectionIndex(1, 3)).toBe(0);
    expect(getPreviousSelectionIndex(0, 3)).toBe(2);
    expect(getPreviousSelectionIndex(-1, 3)).toBe(2);
    expect(getPreviousSelectionIndex(0, 0)).toBe(-1);
  });

  it("maps task and session states to compact status badges", () => {
    expect(getTaskStatusBadge("IN_PROGRESS", "RUNNING")).toMatchObject({ label: "Running" });
    expect(getTaskStatusBadge("REVIEW", "WAITING_FOR_INPUT")).toMatchObject({
      label: "Turn Finished",
    });
    expect(getTaskStatusBadge("TODO", undefined)).toMatchObject({ label: "Todo" });
    expect(getTaskStatusBadge("FAILED", "FAILED")).toMatchObject({
      label: "Failed",
      variant: "destructive",
    });
  });

  it("builds display items from live task metadata with stored fallback", () => {
    const display = buildRecentTaskDisplayItems(
      [recent(CURRENT_TASK_ID, "Stored current"), recent(PREVIOUS_TASK_ID, "Stored previous")],
      buildContext(),
    );

    expect(display[0]).toMatchObject({
      taskId: CURRENT_TASK_ID,
      title: "Current live title",
      isCurrent: true,
      repositoryPath: "kdlbs/kandev",
      workflowName: "Main Flow",
      workflowStepTitle: "Working",
    });
    expect(display[0]?.statusBadge.label).toBe("Running");

    expect(display[1]).toMatchObject({
      taskId: PREVIOUS_TASK_ID,
      title: "Previous live title",
      isCurrent: false,
      repositoryPath: "kdlbs/kandev",
      workflowName: "Review Flow",
      workflowStepTitle: "Review",
    });
  });

  it("builds a recent entry for the active task while preserving prior visit time", () => {
    const previous = recent(CURRENT_TASK_ID, "Old title");

    const entry = buildRecentTaskEntry(CURRENT_TASK_ID, buildContext(), previous);

    expect(entry).toMatchObject({
      taskId: CURRENT_TASK_ID,
      title: "Current live title",
      visitedAt,
      taskState: "IN_PROGRESS",
      sessionState: "RUNNING",
      repositoryPath: "kdlbs/kandev",
      workflowName: "Main Flow",
      workflowStepTitle: "Working",
      workspaceId: WORKSPACE_ID,
    });
  });
});
