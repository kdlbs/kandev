import { applyViewMutationsForTest } from "./sidebar-view-test-helpers";
import { beforeEach, expect, it, vi } from "vitest";
import { workspaceId } from "@/lib/types/ids";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarView } from "./sidebar-view-wire";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
});

it.each(["append", "delete"])(
  "ordinary %s tolerates another tab appending before service admission",
  async (operation) => {
    const base = createDefaultSidebarView("base", "Base");
    const old = createDefaultSidebarView("old", "Delete me");
    const remote = createDefaultSidebarView("remote", "Added elsewhere");
    let persisted = new Map([base, old].map((view) => [view.id, toApiSidebarView(view)]));
    let active = base.id;
    const snapshot = () => ({
      views: [...persisted.values()],
      active_view_id: active,
      draft: null,
    });
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: snapshot() }),
    });
    const admitted = Promise.withResolvers<void>();
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      await admitted.promise;
      const patch = payload.sidebar_view_state!;
      const candidate = new Map(persisted);
      applyViewMutationsForTest(candidate, patch);
      const order = patch.view_order ?? [...candidate.keys()];
      if (order.length !== candidate.size || order.some((id) => !candidate.has(id)))
        throw new ApiError("Invalid collection order", 400, {});
      persisted = new Map(order.map((id) => [id, candidate.get(id)!]));
      active = patch.active_view_id ?? active;
      return {
        settings: {
          user_id: "default-user",
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: 2,
          sidebar_views_by_workspace: { ws: snapshot() },
        },
      } satisfies UserSettingsResponse;
    });
    const created = operation === "append" ? store.getState().createSidebarView() : null;
    if (operation === "delete") store.getState().deleteSidebarView(old.id);
    persisted.set(remote.id, toApiSidebarView(remote));
    admitted.resolve();
    await store.getState().waitForSidebarViewSync("ws");
    expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBeNull();
    expect(persisted.has(remote.id)).toBe(true);
    expect(persisted.has(old.id)).toBe(operation !== "delete");
    if (created) expect(persisted.has(created)).toBe(true);
    const reloaded = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: snapshot() }),
    });
    expect(reloaded.getState().sidebarViewsByWorkspace.ws.views.map((view) => view.id)).toEqual([
      ...persisted.keys(),
    ]);
  },
);
