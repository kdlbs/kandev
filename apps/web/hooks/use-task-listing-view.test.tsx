import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import {
  TASK_LISTING_VIEW_CHANGE_EVENT,
  TASK_LISTING_VIEW_STORAGE_KEY,
} from "@/lib/task-listing/view-preference";

const mocks = vi.hoisted(() => ({
  isMobile: false,
  state: null as AppState | null,
  setUserSettings: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: AppState) => T) => selector(mocks.state!),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import { useTaskListingView } from "./use-task-listing-view";

function resetMocks() {
  mocks.isMobile = false;
  mocks.state = structuredClone(defaultState) as AppState;
  mocks.state.setUserSettings = mocks.setUserSettings;
  mocks.setUserSettings.mockReset();
  window.localStorage.clear();
}

afterEach(resetMocks);
beforeEach(resetMocks);

describe("useTaskListingView", () => {
  it("updates from local custom and storage events", () => {
    const { result } = renderHook(() => useTaskListingView());
    expect(result.current.preferredView).toBe("kanban");

    act(() => {
      window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"pipeline"');
      window.dispatchEvent(new CustomEvent(TASK_LISTING_VIEW_CHANGE_EVENT));
    });
    expect(result.current.preferredView).toBe("pipeline");

    act(() => {
      window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"list"');
      const storageEvent = new Event("storage");
      Object.defineProperty(storageEvent, "key", { value: TASK_LISTING_VIEW_STORAGE_KEY });
      window.dispatchEvent(storageEvent);
    });
    expect(result.current.preferredView).toBe("list");
  });

  it("syncs mobile Pipeline fallback as Kanban without overwriting the saved preference", async () => {
    mocks.isMobile = true;
    mocks.state!.userSettings.kanbanViewMode = "graph2";
    window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"pipeline"');

    const { result } = renderHook(() => useTaskListingView());

    expect(result.current.preferredView).toBe("pipeline");
    expect(result.current.effectiveView).toBe("kanban");
    await waitFor(() => {
      expect(mocks.setUserSettings).toHaveBeenCalledWith(
        expect.objectContaining({ kanbanViewMode: null, loaded: true }),
      );
    });
  });
});
