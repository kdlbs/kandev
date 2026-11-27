import { expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { sidebarTaskSource } from "./sidebar-task-source";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

it("uses bounded server reads until active coverage is authoritative", () => {
  const store = createAppStore({ workspaces: { activeId: "workspace", items: [] } });
  expect(sidebarTaskSource(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
  store.setState((state) => {
    state.workspaceContextRead.workspaceId = "workspace";
    state.workspaceContextRead.generation = state.workspaceContextGeneration;
    state.workflows.taskWorkflowCoverage = {
      workspace_id: "workspace",
      workflow_ids: [],
      complete: true,
    };
    state.repositories.itemsByWorkspaceId.workspace = [];
  });
  expect(sidebarTaskSource(store.getState(), "workspace", DEFAULT_VIEW)).toEqual([]);
  expect(
    sidebarTaskSource(store.getState(), "workspace", {
      ...DEFAULT_VIEW,
      group: "future",
    } as unknown as SidebarView),
  ).toBeNull();
  store.getState().denyTaskOverviewAccess();
  expect(sidebarTaskSource(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
});
