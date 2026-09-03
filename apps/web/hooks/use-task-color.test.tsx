import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { TASK_COLORS_STORAGE_KEY } from "@/lib/task-colors";
import type { UserSettingsResponse } from "@/lib/types/http";
import { useSetTaskColor, useTaskColor } from "./use-task-color";

const mockUpdateUserSettings = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: mockUpdateUserSettings,
}));

function response(
  colors: Record<string, "red" | "orange" | "yellow" | "green" | "blue" | "purple" | "pink" | null>,
  revision: number,
): UserSettingsResponse {
  return {
    settings: {
      user_id: "default-user",
      workspace_id: "" as UserSettingsResponse["settings"]["workspace_id"],
      repository_ids: [],
      sidebar_task_colors: colors,
      revision,
      updated_at: "2026-09-03T00:00:00Z",
    },
    shell_options: [],
  };
}

let capturedStore: ReturnType<typeof useAppStoreApi> | null = null;

function CaptureStore() {
  capturedStore = useAppStoreApi();
  return null;
}

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultState.userSettings,
          loaded: true,
          revision: 1,
          sidebarTaskColors: { "task-1": "red" },
        },
      }}
    >
      <ToastProvider>
        <CaptureStore />
        {children}
      </ToastProvider>
    </StateProvider>
  );
}

beforeEach(() => {
  mockUpdateUserSettings.mockReset();
  capturedStore = null;
  window.localStorage.clear();
});

afterEach(() => {
  cleanup();
});

describe("useTaskColor", () => {
  it("reads the confirmed server-backed color instead of legacy browser storage", () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, JSON.stringify({ "task-1": "pink" }));
    const { result } = renderHook(() => useTaskColor("task-1"), { wrapper });

    expect(result.current).toBe("red");
  });

  it("optimistically sends one narrow normal patch and adopts its response", async () => {
    mockUpdateUserSettings.mockResolvedValue(response({ "task-1": "blue" }, 2));
    const { result } = renderHook(
      () => ({ color: useTaskColor("task-1"), setColor: useSetTaskColor() }),
      { wrapper },
    );

    act(() => result.current.setColor("task-1", "blue"));
    expect(result.current.color).toBe("blue");
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
    expect(mockUpdateUserSettings).toHaveBeenCalledWith({
      sidebar_task_color_patch: {
        colors: { "task-1": "blue" },
        if_missing: false,
      },
    });
    await waitFor(() => expect(result.current.color).toBe("blue"));
  });

  it("rolls back a failed write to the confirmed color and reports a localized error", async () => {
    mockUpdateUserSettings.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(
      () => ({ color: useTaskColor("task-1"), setColor: useSetTaskColor() }),
      { wrapper },
    );

    act(() => result.current.setColor("task-1", "blue"));
    expect(result.current.color).toBe("blue");
    await waitFor(() => expect(result.current.color).toBe("red"));
  });

  it("does not let a delayed stale response replace a newer settings event", async () => {
    let resolveUpdate: ((value: UserSettingsResponse) => void) | undefined;
    mockUpdateUserSettings.mockReturnValue(
      new Promise<UserSettingsResponse>((resolve) => {
        resolveUpdate = resolve;
      }),
    );
    const { result } = renderHook(
      () => ({ color: useTaskColor("task-1"), setColor: useSetTaskColor() }),
      { wrapper },
    );

    act(() => result.current.setColor("task-1", "blue"));
    act(() => {
      capturedStore?.setState((state) => ({
        ...state,
        userSettings: {
          ...state.userSettings,
          revision: 3,
          sidebarTaskColors: { "task-1": "green" },
        },
      }));
    });
    expect(result.current.color).toBe("green");

    act(() => resolveUpdate?.(response({ "task-1": "blue" }, 2)));
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
    expect(result.current.color).toBe("green");
  });
});
