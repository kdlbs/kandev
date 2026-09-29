import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { useEffectiveSidebarView } from "./use-effective-sidebar-view";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: null as string | null },
    sidebarViewsByWorkspace: {},
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

describe("useEffectiveSidebarView", () => {
  it("returns the default view while no workspace view is available", () => {
    const { result } = renderHook(() => useEffectiveSidebarView(null));

    expect(result.current).toEqual(DEFAULT_VIEW);
  });
});
