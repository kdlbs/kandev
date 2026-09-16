import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useSidebarViewsSync } from "./use-sidebar-views-sync";

const mockToast = vi.fn();

type MockState = {
  workspaces: { activeId: string };
  sidebarViewsByWorkspace: { ws: { syncError: string | null } };
  sidebarTaskPrefs: { syncError?: string | null };
  clearSidebarSyncError: () => void;
  clearSidebarTaskPrefsSyncError: () => void;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

describe("useSidebarViewsSync", () => {
  beforeEach(() => {
    mockToast.mockReset();
    mockState = {
      workspaces: { activeId: "ws" },
      sidebarViewsByWorkspace: { ws: { syncError: null } },
      sidebarTaskPrefs: { syncError: null },
      clearSidebarSyncError: vi.fn(),
      clearSidebarTaskPrefsSyncError: vi.fn(),
    };
  });

  it("toasts and clears task preference sync errors", async () => {
    mockState.sidebarTaskPrefs.syncError = "backend unavailable";

    renderHook(() => useSidebarViewsSync());

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith({
        title: "Sidebar task preferences",
        description: "backend unavailable",
        variant: "error",
      });
      expect(mockState.clearSidebarTaskPrefsSyncError).toHaveBeenCalled();
    });
  });

  it("toasts and clears sidebar view sync errors", async () => {
    mockState.sidebarViewsByWorkspace.ws.syncError = "boom";

    renderHook(() => useSidebarViewsSync());

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith({
        title: "Sidebar views",
        description: "boom",
        variant: "error",
      });
      expect(mockState.clearSidebarSyncError).toHaveBeenCalled();
    });
  });
});
