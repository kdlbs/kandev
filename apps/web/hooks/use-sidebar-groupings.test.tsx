import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { useSidebarGroupings } from "./use-sidebar-groupings";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(async () => ({ settings: {} })),
}));
const choice = { reference: "plugin:acme:labels" as const, label: "Labels" };
function Wrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{ workspaces: { items: [], activeId: "a" }, workspaceContextGeneration: 1 }}
    >
      {children}
    </StateProvider>
  );
}
afterEach(() => {
  cleanup();
  pluginRegistry.setSidebarGroupings([]);
  vi.clearAllMocks();
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
it.each(["workspace ABA", "authorization"])(
  "an old selector callback cannot write after %s changes",
  (kind) => {
    pluginRegistry.setSidebarGroupings([{ ...choice, generation: "installation" }]);
    const { result } = renderHook(
      () => ({ choices: useSidebarGroupings(), store: useAppStoreApi() }),
      { wrapper: Wrapper },
    );
    const select = result.current.choices.selectGrouping;
    const store = result.current.store;
    act(() => {
      if (kind === "workspace ABA") {
        store.getState().setActiveWorkspace("b");
        store.getState().setActiveWorkspace("a");
      } else
        store.setState((state) => ({
          auth: { ...state.auth, authenticated: !state.auth.authenticated },
        }));
    });
    const before = structuredClone(selectSidebarViews(store.getState()));
    act(() => {
      select(choice);
    });
    expect(selectSidebarViews(store.getState())).toEqual(before);
  },
);
