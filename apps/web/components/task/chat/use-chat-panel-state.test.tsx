import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task, type WorkflowStep } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  documentPanel: { activeDocumentBySessionId: Record<string, unknown> };
  chatInput: { planModeBySessionId: Record<string, boolean> };
  taskSessions: { items: Record<string, { metadata?: Record<string, unknown> | null }> };
  kanban: { tasks: KanbanState["tasks"]; steps: KanbanState["steps"] };
  setActiveDocument: ReturnType<typeof vi.fn>;
  setPlanMode: ReturnType<typeof vi.fn>;
};

let mockState: MockState;
const applyBuiltInPreset = vi.fn();
const addContextFile = vi.fn();
const TIMESTAMP = "2026-01-01T00:00:00Z";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: () => vi.fn(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => applyBuiltInPreset,
}));

vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      addFile: addContextFile,
      removeFile: vi.fn(),
    }),
}));

vi.mock("@/components/task/chat/use-plan-mode-helpers", () => ({
  useAutoDisablePlanMode: vi.fn(),
  usePlanLayoutHandlers: () => ({
    togglePlanLayout: vi.fn(),
    handlePlanModeChange: vi.fn(),
  }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn(() => new Promise(() => {})),
}));

import { usePlanMode } from "./use-chat-panel-state";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-plan",
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

function makeStep(): WorkflowStep {
  return {
    id: "step-plan",
    workflow_id: workflowId("wf-1"),
    name: "Plan",
    position: 1,
    color: "bg-blue-500",
    events: { on_enter: [{ type: "enable_plan_mode" }] },
    allow_manual_move: true,
    prompt: "",
    is_start_step: false,
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

describe("usePlanMode", () => {
  beforeEach(() => {
    applyBuiltInPreset.mockClear();
    addContextFile.mockClear();
    mockState = {
      documentPanel: { activeDocumentBySessionId: {} },
      chatInput: { planModeBySessionId: {} },
      taskSessions: { items: { "session-1": { metadata: { plan_mode: true } } } },
      kanban: { tasks: [], steps: [] },
      setActiveDocument: vi.fn(),
      setPlanMode: vi.fn(),
    };
  });

  it("auto-applies plan layout from task detail and workflow-step Query data", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());
    client.setQueryData(qk.workflows.steps("wf-1"), [makeStep()]);

    renderHook(() => usePlanMode("session-1", "task-1"), {
      wrapper: wrapper(client),
    });

    await waitFor(() => expect(applyBuiltInPreset).toHaveBeenCalledWith("plan"));
    expect(mockState.setActiveDocument).toHaveBeenCalledWith("session-1", {
      type: "plan",
      taskId: "task-1",
    });
    expect(addContextFile).toHaveBeenCalledWith("session-1", {
      path: "plan:context",
      name: "Plan",
    });
  });
});
