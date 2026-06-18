import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useSidebarViewsSync } from "./use-sidebar-views-sync";

const mockToast = vi.fn();

type MockState = {
  sidebarViews: { views: Array<{ id: string }>; syncError: string | null };
  sidebarTaskPrefs: { syncError?: string | null };
  userSettings: { loaded: boolean };
  migrateLocalViewsToBackend: () => void;
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
      sidebarViews: { views: [], syncError: null },
      sidebarTaskPrefs: { syncError: null },
      userSettings: { loaded: true },
      migrateLocalViewsToBackend: vi.fn(),
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
});
