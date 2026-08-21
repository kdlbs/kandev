import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useHomeAffordance } from "./use-home-affordance";

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  appSidebar: { settingsMode: false, collapsed: false },
};

let inOffice = false;
let modeUnknown = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => inOffice,
  useOfficeModeState: () => {
    if (modeUnknown) return "unknown";
    return inOffice ? "office" : "kanban";
  },
}));

describe("useHomeAffordance", () => {
  beforeEach(() => {
    state.workspaces.activeId = "ws-1";
    state.appSidebar.settingsMode = false;
    state.appSidebar.collapsed = false;
    inOffice = false;
    modeUnknown = false;
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

  it("lands on the office dashboard for an office workspace", () => {
    inOffice = true;

    const { result } = renderHook(() => useHomeAffordance());

    // Carries the workspace id so this is byte-identical to the brand link's
    // `workspaceHomeHref`. The two render side by side in the sidebar header,
    // and pointing at different URLs was the original "two homes" defect.
    expect(result.current.href).toBe("/office?workspaceId=ws-1");
  });

  it("hides Home until workspace mode resolves", () => {
    modeUnknown = true;

    const { result } = renderHook(() => useHomeAffordance());

    expect(result.current.mode).toBe("none");
    expect(result.current.href).toBe("");
  });
});
