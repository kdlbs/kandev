import { describe, expect, it } from "vitest";
import { DEFAULT_VIEW } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces, selectSidebarViews } from "./sidebar-workspace-state";

describe("workspace sidebar view fallback", () => {
  it("resolves an empty saved collection to the canonical All tasks view", () => {
    const mapped = mapSidebarWorkspaces({
      workspace: { views: [], active_view_id: "missing", draft: null },
    });

    expect(mapped.workspace).toMatchObject({
      views: [DEFAULT_VIEW],
      activeViewId: DEFAULT_VIEW.id,
      draft: null,
    });
  });

  it("uses All tasks for an empty workspace state without borrowing another workspace", () => {
    const selected = selectSidebarViews({
      workspaces: { activeId: "workspace-a" },
      sidebarViewsByWorkspace: {
        "workspace-a": { views: [], activeViewId: "", draft: null, syncError: null },
        "workspace-b": {
          views: [{ ...DEFAULT_VIEW, id: "workspace-b-view", name: "Workspace B" }],
          activeViewId: "workspace-b-view",
          draft: null,
          syncError: null,
        },
      },
    });

    expect(selected.views).toEqual([DEFAULT_VIEW]);
    expect(selected.activeViewId).toBe(DEFAULT_VIEW.id);
  });
});
