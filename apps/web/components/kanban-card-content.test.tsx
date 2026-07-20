import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Task as HttpTask,
} from "@/lib/types/http";
import { KanbanCardBody } from "./kanban-card-content";
import type { Task } from "./kanban-card";

vi.mock("@/components/github/pr-task-icon", () => ({
  PRTaskIcon: () => null,
}));

const CREATED_AT = "2026-06-24T00:00:00Z";
const PARENT_TASK_ID = toTaskId("parent-1");
const WORKSPACE_ID = toWorkspaceId("workspace-1");
const WORKFLOW_ID = toWorkflowId("workflow-1");

function queryClientWithParent() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(qk.tasks.detail(PARENT_TASK_ID), parentTask());
  return client;
}

function parentTask(): HttpTask {
  return {
    id: PARENT_TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: "step-1",
    position: 0,
    title: "Query parent task",
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [],
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function subtask(): Task {
  return {
    id: "task-1",
    workflowStepId: "step-1",
    title: "Subtask",
    description: "",
    position: 0,
    state: "TODO",
    parentTaskId: "parent-1",
  };
}

function renderBody() {
  return render(
    <QueryClientProvider client={queryClientWithParent()}>
      <StateProvider>
        <KanbanCardBody task={subtask()} repositoryChips={[]} />
      </StateProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => cleanup());

describe("KanbanCardBody", () => {
  it("uses Query task detail for the subtask parent badge when legacy kanban is empty", () => {
    renderBody();

    expect(screen.getByText("Query parent task")).toBeTruthy();
  });
});
