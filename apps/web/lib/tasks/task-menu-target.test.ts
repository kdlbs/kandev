import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { resolveTaskMenuTarget } from "./task-menu-target";

const identity = { taskId: "A", workspaceId: "workspace" };
function fixture(loss?: string) {
  const store = createAppStore();
  store.setState((state) => ({
    workspaces: { ...state.workspaces, activeId: loss === "workspace" ? "elsewhere" : "workspace" },
    workflows: {
      ...state.workflows,
      items:
        loss === "workflow" ? [] : [{ id: "workflow", name: "Flow", workspaceId: "workspace" }],
    },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Flow",
          steps: [],
          tasks:
            loss === "missing" || loss === "archived"
              ? []
              : [
                  {
                    id: "A",
                    title: "Renamed",
                    workflowId: "workflow",
                    workflowStepId: "step",
                    position: 0,
                    priority: "high",
                    primaryExecutorType: "worktree",
                  },
                ],
        },
      },
    },
  }));
  return store;
}
describe("task menu eligibility", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-002.1, AC-TASKS-THREADS-ACTIONS-002.2
  it("resolves live A metadata independently of selected B/session", () => {
    const store = fixture();
    store.getState().setActiveSession("B", "session-B");
    const state = store.getState();
    expect(resolveTaskMenuTarget(state, identity)).toMatchObject({
      id: "A",
      title: "Renamed",
      priority: "high",
      remoteExecutorType: "worktree",
      workspaceId: "workspace",
    });
  });
  it.each(["workspace", "missing", "archived", "workflow"] as const)(
    "fails closed on %s loss",
    (loss) => {
      const state = fixture(loss).getState();
      expect(resolveTaskMenuTarget(state, identity)).toBeNull();
    },
  );
});
