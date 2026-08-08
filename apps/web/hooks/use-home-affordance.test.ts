import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useHomeAffordance } from "./use-home-affordance";

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  appSidebar: { settingsMode: false, collapsed: false },
};

let inOffice = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => inOffice,
}));

describe("useHomeAffordance", () => {
  beforeEach(() => {
    state.workspaces.activeId = "ws-1";
    state.appSidebar.settingsMode = false;
    state.appSidebar.collapsed = false;
    inOffice = false;
  });

  it("defaults to a phone-only crumb pointing at the workspace overview", () => {
    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.mode).toBe("phone");
    expect(result.current.href).toBe("/?home=overview&workspaceId=ws-1");
  });

  it("drops the workspace param when no workspace is active", () => {
    state.workspaces.activeId = null;

    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.href).toBe("/?home=overview");
  });

  it("returns none for root-variant pages", () => {
    const { result } = renderHook(() => useHomeAffordance("root"));

    expect(result.current.mode).toBe("none");
  });

  it("returns always while the expanded sidebar shows the settings tree", () => {
    state.appSidebar.settingsMode = true;

    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.mode).toBe("always");
  });

  it("stays phone-only when settings mode is on but the sidebar is collapsed", () => {
    // The takeover only renders expanded; the collapsed rail keeps its icons.
    state.appSidebar.settingsMode = true;
    state.appSidebar.collapsed = true;

    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.mode).toBe("phone");
  });

  it("lands on the office dashboard inside Office", () => {
    inOffice = true;

    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.href).toBe("/office");
  });
});
