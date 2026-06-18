import { describe, expect, it } from "vitest";

import { hasHydratedKanbanRouteState } from "./kanban-route-hydration";
import type { AppState } from "@/lib/state/store";

type HydrationState = Pick<AppState, "kanban" | "kanbanMulti" | "workflows" | "workspaces">;

function state(overrides: Partial<HydrationState> = {}): HydrationState {
  return {
    workspaces: { activeId: "ws-1", items: [] },
    workflows: {
      activeId: "wf-1",
      items: [{ id: "wf-1", workspaceId: "ws-1", name: "Development" }],
    },
    kanban: { workflowId: "wf-1", steps: [], tasks: [] },
    kanbanMulti: {
      snapshots: {
        "wf-1": { workflowId: "wf-1", workflowName: "Development", steps: [], tasks: [] },
      },
      isLoading: false,
    },
    ...overrides,
  };
}

describe("hasHydratedKanbanRouteState", () => {
  it("accepts boot-hydrated state for the active workspace and workflow", () => {
    expect(hasHydratedKanbanRouteState(state(), {})).toBe(true);
    expect(hasHydratedKanbanRouteState(state(), { workspaceId: "ws-1", workflowId: "wf-1" })).toBe(
      true,
    );
  });

  it("rejects missing or mismatched route state so the client can fetch", () => {
    expect(
      hasHydratedKanbanRouteState(state({ workspaces: { activeId: null, items: [] } }), {}),
    ).toBe(false);
    expect(hasHydratedKanbanRouteState(state(), { workspaceId: "ws-2" })).toBe(false);
    expect(hasHydratedKanbanRouteState(state(), { workflowId: "wf-2" })).toBe(false);
    expect(
      hasHydratedKanbanRouteState(
        state({
          kanban: { workflowId: null, steps: [], tasks: [] },
          kanbanMulti: { snapshots: {}, isLoading: false },
        }),
        {},
      ),
    ).toBe(false);
  });
});
