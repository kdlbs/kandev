import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { workspaceId } from "@/lib/types/ids";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));
beforeEach(() => vi.clearAllMocks());
const sidebarSaveFailedMessage = "A save failed";

it("captures workspace identity for every saved-view mutation", () => {
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().createSidebarView();
  expect(updateUserSettings).toHaveBeenCalledWith(
    expect.objectContaining({
      sidebar_view_state: expect.objectContaining({ workspace_id: "a" }),
    }),
  );
  store.getState().setActiveWorkspace("b");
  store.getState().createSidebarView();
  expect(updateUserSettings).toHaveBeenLastCalledWith(
    expect.objectContaining({
      sidebar_view_state: expect.objectContaining({ workspace_id: "b" }),
    }),
  );
});

it("does not mutate sidebar views without an active workspace", () => {
  const store = createAppStore();
  expect(store.getState().createSidebarView()).toBeNull();
  expect(updateUserSettings).not.toHaveBeenCalled();
});

it("hydrates distinct workspace collections and preserves them while switching", () => {
  const view = (name: string) => ({
    id: "shared",
    name,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    collapsed_groups: [],
  });
  const store = createAppStore({
    workspaces: { items: [], activeId: "a" },
    userSettings: {
      sidebarViewsByWorkspace: {
        a: { views: [view("A")], active_view_id: "shared", draft: null },
        b: { views: [view("B")], active_view_id: "shared", draft: null },
      },
    },
  } as unknown as Parameters<typeof createAppStore>[0]);
  expect(store.getState().sidebarViewsByWorkspace.a?.views[0].name).toBe("A");
  store.getState().setActiveWorkspace("b");
  store.getState().renameSidebarView("shared", "B edited");
  expect(store.getState().sidebarViewsByWorkspace.a.views[0].name).toBe("A");
  expect(store.getState().sidebarViewsByWorkspace.b.views[0].name).toBe("B edited");
});

it("keeps pending workspace edits when a different workspace broadcasts settings", async () => {
  const { registerUsersHandlers } = await import("@/lib/ws/handlers/users");
  let resolve!: (value: Awaited<ReturnType<typeof updateUserSettings>>) => void;
  vi.mocked(updateUserSettings).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  const created = store.getState().createSidebarView();
  const handlers = registerUsersHandlers(store);
  handlers["user.settings.updated"]!({
    type: "user.settings.updated",
    payload: {
      revision: 100,
      sidebar_views_by_workspace: {
        a: {
          views: [
            {
              id: "original",
              name: "Old server A",
              filters: [],
              sort: { key: "state", direction: "asc" },
              group: "none",
              collapsed_groups: [],
            },
          ],
          active_view_id: "original",
          draft: null,
        },
      },
    },
  } as never);
  expect(store.getState().sidebarViewsByWorkspace.a.activeViewId).toBe(created);
  resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
});

it("projects workspace views loaded through the HTTP settings setter", () => {
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().setUserSettings({
    ...store.getState().userSettings,
    sidebarViewsByWorkspace: {
      a: {
        views: [
          {
            id: "http",
            name: "HTTP view",
            filters: [],
            sort: { key: "state", direction: "asc" },
            group: "none",
            collapsed_groups: [],
          },
        ],
        active_view_id: "http",
        draft: null,
      },
    },
  });
  expect(store.getState().sidebarViewsByWorkspace.a?.activeViewId).toBe("http");
});

it("rolls a delayed failure back only in its originating workspace", async () => {
  const { waitFor } = await import("@testing-library/react");
  const { ApiError } = await import("@/lib/api/client");
  let reject!: (error: Error) => void;
  vi.mocked(updateUserSettings).mockImplementationOnce(
    () =>
      new Promise((_resolve, fail) => {
        reject = fail;
      }),
  );
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().createSidebarView();
  store.getState().setActiveWorkspace("b");
  const b = store.getState().createSidebarView();
  reject(new ApiError(sidebarSaveFailedMessage, 500, {}));
  await waitFor(() =>
    expect(store.getState().sidebarViewsByWorkspace.a.syncError).toBe(sidebarSaveFailedMessage),
  );
  expect(store.getState().sidebarViewsByWorkspace.b.activeViewId).toBe(b);
  expect(store.getState().sidebarViewsByWorkspace.b.views).toHaveLength(2);
  store.getState().setActiveWorkspace("a");
  expect(store.getState().sidebarViewsByWorkspace.a.views).toHaveLength(1);
});

it("rolls a queued failure back after returning to its originating workspace", async () => {
  const { waitFor } = await import("@testing-library/react");
  const { ApiError } = await import("@/lib/api/client");
  let rejectA!: (error: Error) => void;
  vi.mocked(updateUserSettings)
    .mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectA = reject;
        }),
    )
    .mockResolvedValue({ settings: {} } as never);

  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().createSidebarView();
  store.getState().setActiveWorkspace("b");
  store.getState().createSidebarView();
  store.getState().setActiveWorkspace("a");
  rejectA(new ApiError(sidebarSaveFailedMessage, 500, {}));

  await waitFor(() =>
    expect(store.getState().sidebarViewsByWorkspace.a.syncError).toBe(sidebarSaveFailedMessage),
  );
  expect(store.getState().sidebarViewsByWorkspace.a.views).toHaveLength(1);
  expect(store.getState().sidebarViewsByWorkspace.b.views).toHaveLength(2);
});

it("applies a queued success after returning to its originating workspace", async () => {
  const { waitFor } = await import("@testing-library/react");
  let resolveA!: (value: Awaited<ReturnType<typeof updateUserSettings>>) => void;
  vi.mocked(updateUserSettings)
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveA = resolve;
        }),
    )
    .mockResolvedValue({ settings: {} } as never);

  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  const aView = store.getState().createSidebarView();
  store.getState().setActiveWorkspace("b");
  const bView = store.getState().createSidebarView();
  store.getState().setActiveWorkspace("a");
  resolveA({ settings: {} } as never);

  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.a.syncPending).toBe(false));
  expect(store.getState().sidebarViewsByWorkspace.a.activeViewId).toBe(aView);
  expect(store.getState().sidebarViewsByWorkspace.b.activeViewId).toBe(bView);
});

it("restores independent drafts and collapsed groups when returning to a workspace", () => {
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().toggleSidebarGroupCollapsed("view-all-tasks", "repo-a");
  store.getState().updateSidebarDraft({ group: "state" });
  store.getState().setActiveWorkspace("b");
  store.getState().updateSidebarDraft({ group: "none" });
  store.getState().setActiveWorkspace("a");
  expect(store.getState().sidebarViewsByWorkspace.a.draft?.group).toBe("state");
  expect(store.getState().sidebarViewsByWorkspace.a.views[0].collapsedGroups).toEqual(["repo-a"]);
  expect(store.getState().sidebarViewsByWorkspace.b.draft?.group).toBe("none");
  expect(store.getState().sidebarViewsByWorkspace.b.views[0].collapsedGroups).toEqual([]);
});

it("preserves an acknowledged save against an older HTTP settings snapshot", async () => {
  const { waitFor } = await import("@testing-library/react");
  const view = {
    id: "view-all-tasks",
    name: "Acknowledged",
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    collapsed_groups: [],
  };
  vi.mocked(updateUserSettings).mockResolvedValueOnce({
    settings: {
      user_id: "user",
      workspace_id: workspaceId("a"),
      repository_ids: [],
      updated_at: "2026-09-15T00:00:00Z",
      revision: 20,
      sidebar_views_by_workspace: { a: { views: [view], active_view_id: view.id, draft: null } },
    },
  });
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  store.getState().renameSidebarView(view.id, "Acknowledged");
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.a.syncPending).toBe(false));
  store.getState().setUserSettings({
    ...store.getState().userSettings,
    revision: 19,
    sidebarViewsByWorkspace: {
      a: { views: [{ ...view, name: "Stale" }], active_view_id: view.id, draft: null },
    },
  });
  expect(store.getState().sidebarViewsByWorkspace.a.views[0].name).toBe("Acknowledged");
});
