import { describe, expect, it } from "vitest";

import { hasHydratedKanbanRouteState } from "./kanban-route-hydration";
import type { AppState } from "@/lib/state/store";
import type { WorkflowItem } from "@/lib/state/slices";

type HydrationState = Pick<AppState, "workflows" | "workspaces">;
const workflows: WorkflowItem[] = [{ id: "wf-1", workspaceId: "ws-1", name: "Development" }];

function state(overrides: Partial<HydrationState> = {}): HydrationState {
  return {
    workspaces: { activeId: "ws-1", items: [] },
    workflows: { activeId: "wf-1" },
    ...overrides,
  };
}

describe("hasHydratedKanbanRouteState", () => {
  it("accepts query-hydrated snapshots for the active workspace and workflow", () => {
    expect(hasHydratedKanbanRouteState(state(), {}, workflows, new Set(["wf-1"]))).toBe(true);
    expect(
      hasHydratedKanbanRouteState(
        state(),
        { workspaceId: "ws-1", workflowId: "wf-1" },
        workflows,
        new Set(["wf-1"]),
      ),
    ).toBe(true);
  });

  it("rejects missing or mismatched route state so the client can fetch", () => {
    expect(
      hasHydratedKanbanRouteState(
        state({ workspaces: { activeId: null, items: [] } }),
        {},
        workflows,
        new Set(["wf-1"]),
      ),
    ).toBe(false);
    expect(
      hasHydratedKanbanRouteState(state(), { workspaceId: "ws-2" }, workflows, new Set(["wf-1"])),
    ).toBe(false);
    expect(
      hasHydratedKanbanRouteState(state(), { workflowId: "wf-2" }, workflows, new Set(["wf-1"])),
    ).toBe(false);
    expect(hasHydratedKanbanRouteState(state(), {}, workflows, new Set())).toBe(false);
  });
});
