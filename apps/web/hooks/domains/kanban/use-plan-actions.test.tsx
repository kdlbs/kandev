import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task, type WorkflowStep } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  kanban: { workflowId: string | null; steps: KanbanState["steps"]; tasks: KanbanState["tasks"] };
  tasks: { activeSessionId: string | null };
};

let mockState: MockState;
const TIMESTAMP = "2026-01-01T00:00:00Z";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
  moveTask: vi.fn(),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn(() => new Promise(() => {})),
}));

import { useNextWorkflowStep } from "./use-plan-actions";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-current",
    position: 1,
    title: "Query task",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function makeStep(id: string, name: string, position: number): WorkflowStep {
  return {
    id,
    workflow_id: workflowId("wf-1"),
    name,
    position,
    color: "bg-blue-500",
    events: {},
    allow_manual_move: true,
    prompt: "",
    is_start_step: position === 1,
    show_in_command_panel: true,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

describe("useNextWorkflowStep", () => {
  beforeEach(() => {
    mockState = {
      kanban: { workflowId: null, steps: [], tasks: [] },
      tasks: { activeSessionId: "session-1" },
    };
  });

  it("derives the next step from task detail and workflow-step Query caches", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());
    client.setQueryData(qk.workflows.steps("wf-1"), [
      makeStep("step-current", "Current", 1),
      makeStep("step-next", "Next", 2),
    ]);

    const { result } = renderHook(() => useNextWorkflowStep("task-1"), {
      wrapper: wrapper(client),
    });

    expect(result.current.proceedStepName).toBe("Next");
  });
});
