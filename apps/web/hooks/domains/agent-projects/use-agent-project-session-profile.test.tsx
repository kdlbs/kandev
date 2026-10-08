import { renderHook } from "@testing-library/react";
import { createElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import { useAgentProjectSessionProfile } from "./use-agent-project-session-profile";

const mocks = vi.hoisted(() => ({ list: vi.fn() }));

vi.mock("@/lib/api/domains/agent-projects-api", () => ({
  archiveAgentProject: vi.fn(),
  createAgentProject: vi.fn(),
  deleteAgentProject: vi.fn(),
  listAgentProjects: mocks.list,
  restoreAgentProject: vi.fn(),
  updateAgentProject: vi.fn(),
}));

function renderPolicy(task: Record<string, unknown>) {
  const initialState = {
    features: { agentProjects: false },
    kanban: {
      workflowId: null,
      steps: [],
      tasks: [
        {
          id: "task-1",
          workspaceId: "workspace-1",
          workflowId: "workflow-1",
          workflowStepId: "step-1",
          title: "Task",
          position: 0,
          ...task,
        },
      ],
    },
  } as unknown as HydrationState;

  return renderHook(
    () => useAgentProjectSessionProfile({ taskId: "task-1", workspaceId: "workspace-1" }),
    { wrapper: ({ children }) => createElement(StateProvider, { initialState, children }) },
  );
}

describe("useAgentProjectSessionProfile with Agent Projects disabled", () => {
  it("preserves ordinary task profile selection without querying project data", () => {
    const { result } = renderPolicy({});

    expect(result.current).toEqual({ kind: "ordinary" });
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it("fails closed for a task whose metadata identifies project membership", () => {
    const { result } = renderPolicy({ metadata: { agent_project_id: "project-1" } });

    expect(result.current).toEqual({ kind: "unavailable" });
    expect(mocks.list).not.toHaveBeenCalled();
  });
});
