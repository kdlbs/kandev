import { act, renderHook, cleanup } from "@testing-library/react";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import { defaultSidebarLayout, toApiSidebarLayout } from "@/lib/sidebar/layout-types";
import { toggleNodeVisibility } from "@/lib/sidebar/layout-operations";
import { ApiError } from "@/lib/api/client";

const mocks = vi.hoisted(() => ({ update: vi.fn(), fetch: vi.fn() }));
let state = {
  workspaces: { activeId: "a" },
  userSettings: createDefaultUserSettings(),
  setUserSettings: (next: ReturnType<typeof createDefaultUserSettings>) => {
    state.userSettings = next;
  },
};
let store = { getState: () => state };
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (snapshot: typeof state) => unknown) => select(state),
  useAppStoreApi: () => store,
}));
vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: mocks.update,
  fetchUserSettings: mocks.fetch,
}));
import { useSidebarCustomization } from "./use-sidebar-customization";
beforeEach(() => {
  state = { ...state, workspaces: { activeId: "a" }, userSettings: createDefaultUserSettings() };
  store = { getState: () => state };
  mocks.update.mockReset();
  mocks.fetch.mockReset();
});
afterEach(cleanup);

it("refreshes a conflicting layout and leaves the rejected mutation unapplied", async () => {
  const latest = { ...defaultSidebarLayout(), revision: 7, navigationHeight: 90 };
  mocks.update.mockRejectedValueOnce(new ApiError("conflict", 409, {}));
  mocks.fetch.mockResolvedValue({
    settings: { revision: 9, sidebar_layouts_by_workspace: { a: toApiSidebarLayout(latest) } },
  });
  const { result } = renderHook(() => useSidebarCustomization());
  await act(async () => {
    await result.current.mutate((layout) => toggleNodeVisibility(layout, "home", false));
  });
  expect(result.current.status).toBe("conflict");
  expect(state.userSettings.sidebarLayoutsByWorkspace.a.revision).toBe(7);
  expect(
    state.userSettings.sidebarLayoutsByWorkspace.a.nodes.find((node) => node.id === "home")
      ?.visible,
  ).toBe(true);
  mocks.update.mockResolvedValue({
    settings: {
      revision: 10,
      sidebar_layouts_by_workspace: { a: toApiSidebarLayout({ ...latest, revision: 8 }) },
    },
  });
  await act(async () => {
    await result.current.mutate((layout) => ({ ...layout, navigationExpanded: true }));
  });
  expect(mocks.update.mock.calls[1][0].sidebar_layout_state.expected_revision).toBe(7);
});

it("keeps a late response in its workspace and clears the new workspace status", async () => {
  let resolve: (value: unknown) => void = () => {};
  mocks.update.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result, rerender } = renderHook(() => useSidebarCustomization());
  let pending: Promise<void>;
  act(() => {
    pending = result.current.mutate((layout) => toggleNodeVisibility(layout, "home", false));
  });
  await act(async () => {
    await Promise.resolve();
  });
  state.workspaces = { activeId: "b" };
  rerender();
  expect(result.current.status).toBeNull();
  const saved = toggleNodeVisibility(defaultSidebarLayout(), "home", false);
  await act(async () => {
    resolve({
      settings: {
        revision: 1,
        sidebar_layouts_by_workspace: { a: toApiSidebarLayout({ ...saved, revision: 1 }) },
      },
    });
    await pending;
  });
  expect(state.workspaces.activeId).toBe("b");
  expect(state.userSettings.sidebarLayoutsByWorkspace.b).toBeUndefined();
  expect(
    state.userSettings.sidebarLayoutsByWorkspace.a.nodes.find((node) => node.id === "home")
      ?.visible,
  ).toBe(false);
});

it("recovers a committed layout when its HTTP response is lost", async () => {
  const saved = toggleNodeVisibility(defaultSidebarLayout(), "home", false);
  mocks.update.mockRejectedValueOnce(new Error("response lost"));
  mocks.fetch.mockResolvedValue({
    settings: {
      revision: 3,
      sidebar_layouts_by_workspace: { a: toApiSidebarLayout({ ...saved, revision: 2 }) },
    },
  });
  const { result } = renderHook(() => useSidebarCustomization());
  await act(async () => {
    await result.current.mutate((layout) => toggleNodeVisibility(layout, "home", false));
  });
  expect(result.current.status).toBe("error");
  expect(state.userSettings.sidebarLayoutsByWorkspace.a?.revision).toBe(2);
  expect(
    state.userSettings.sidebarLayoutsByWorkspace.a?.nodes.find((node) => node.id === "home")
      ?.visible,
  ).toBe(false);
});

it("retains acknowledged preferences and reports the original error if refresh also fails", async () => {
  mocks.update.mockRejectedValueOnce(new Error("offline"));
  mocks.fetch.mockRejectedValueOnce(new Error("offline"));
  const { result } = renderHook(() => useSidebarCustomization());
  await act(async () => {
    await result.current.preferences({ sidebar_fast_actions_enabled: true });
  });
  expect(mocks.fetch).toHaveBeenCalledOnce();
  expect(result.current.status).toBe("error");
  expect(state.userSettings.sidebarFastActionsEnabled).toBe(false);
});
