import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { useKanbanData } from "./use-kanban-data";

const api = vi.hoisted(() => ({
  fetchWorkflowSnapshot: vi.fn().mockResolvedValue({
    workflow: { id: "workflow", workspace_id: "workspace" },
    tasks: [],
    steps: [],
  }),
}));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  ...api,
}));
vi.mock("@/hooks/use-user-display-settings", () => ({
  useUserDisplaySettings: () => ({
    settings: defaultState.userSettings,
    commitSettings: vi.fn(),
    selectedRepositoryIds: [],
  }),
}));

describe("board snapshot ownership", () => {
  it("reads selected board projections without fetching outside panel demand", async () => {
    renderHook(() => useKanbanData({ onWorkspaceChange: vi.fn(), onWorkflowChange: vi.fn() }), {
      wrapper: ({ children }) => (
        <StateProvider
          initialState={{
            workflows: {
              ...defaultState.workflows,
              activeId: "workflow",
              items: [{ id: "workflow", name: "Board", workspaceId: "workspace" }],
            },
            workspaces: { ...defaultState.workspaces, activeId: "workspace" },
          }}
        >
          {children}
        </StateProvider>
      ),
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(api.fetchWorkflowSnapshot).not.toHaveBeenCalled();
  });
});
