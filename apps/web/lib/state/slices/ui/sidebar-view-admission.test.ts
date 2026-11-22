import { waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { workspaceId } from "@/lib/types/ids";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarView } from "./sidebar-view-wire";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";
import { applyViewMutationsForTest } from "./sidebar-view-test-helpers";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
});

it.each(["newer rename event", "newer delete event", "deletion before update admission"])(
  "admits queued disclosure against authoritative state: %s",
  async (schedule) => {
    const base = createDefaultSidebarView("base", "Fallback");
    const view = {
      ...createDefaultSidebarView("grouped", "Original"),
      group: "plugin:tags:tags" as const,
    };
    const persisted = new Map([base, view].map((item) => [item.id, toApiSidebarView(item)]));
    let active = view.id;
    let revision = 9;
    const snapshot = () => ({
      views: structuredClone([...persisted.values()]),
      active_view_id: active,
      draft: null,
    });
    const acknowledge = (): UserSettingsResponse => ({
      settings: {
        user_id: "default-user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision,
        sidebar_views_by_workspace: { ws: snapshot() },
      },
    });
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: snapshot() }, {}, revision),
    });
    store.setState((state) => ({ userSettings: { ...state.userSettings, revision } }));
    const first = Promise.withResolvers<UserSettingsResponse>();
    let initial: UserSettingsResponse | undefined;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      applyViewMutationsForTest(persisted, patch);
      if (patch.active_view_id) active = patch.active_view_id;
      revision++;
      if (!initial) {
        initial = acknowledge();
        return first.promise;
      }
      return acknowledge();
    });
    store.getState().renameSidebarView(view.id, "Local rename");
    await waitFor(() => expect(initial).toBeDefined());
    store.getState().toggleSidebarGroupCollapsed(view.id, "kept-disclosure");
    if (schedule === "newer rename event")
      persisted.set(view.id, { ...persisted.get(view.id)!, name: "Remote rename" });
    else {
      persisted.delete(view.id);
      active = base.id;
    }
    revision++;
    if (schedule !== "deletion before update admission") {
      registerUsersHandlers(store)["user.settings.updated"]!({
        type: "notification",
        action: "user.settings.updated",
        payload: {
          user_id: "default",
          workspace_id: "ws",
          repository_ids: [],
          revision,
          sidebar_views_by_workspace: { ws: snapshot() },
        },
      });
    }
    first.resolve(initial!);
    await store.getState().waitForSidebarViewSync("ws");
    const reloaded = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: snapshot() }, {}, revision),
    }).getState().sidebarViewsByWorkspace.ws;
    if (schedule === "newer rename event") {
      expect(persisted.get(view.id)).toMatchObject({
        name: "Remote rename",
        collapsed_groups: ["kept-disclosure"],
      });
      expect(reloaded.views.find((item) => item.id === view.id)?.name).toBe("Remote rename");
    } else {
      expect(persisted.has(view.id)).toBe(false);
      expect(reloaded.activeViewId).toBe(base.id);
      expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(base.id);
    }
  },
);
