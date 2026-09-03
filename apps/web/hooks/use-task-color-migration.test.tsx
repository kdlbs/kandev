import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { TASK_COLORS_STORAGE_KEY } from "@/lib/task-colors";
import type { UserSettingsResponse } from "@/lib/types/http";
import { useTaskColorMigration } from "./use-task-color-migration";

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
          sidebarTaskColors: { "task-existing": "red", "task-cleared": null },
        },
      }}
    >
      <CaptureStore />
      {children}
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

describe("useTaskColorMigration", () => {
  it("imports valid legacy values only after settings are loaded and removes the key after success", async () => {
    window.localStorage.setItem(
      TASK_COLORS_STORAGE_KEY,
      JSON.stringify({
        "task-existing": "blue",
        "task-cleared": "pink",
        "task-new": "green",
        "task-invalid": "gray",
      }),
    );
    mockUpdateUserSettings.mockResolvedValue(
      response({ "task-existing": "red", "task-cleared": null, "task-new": "green" }, 2),
    );

    renderHook(() => useTaskColorMigration(true), { wrapper });

    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
    expect(mockUpdateUserSettings).toHaveBeenCalledWith({
      sidebar_task_color_patch: {
        colors: {
          "task-existing": "blue",
          "task-cleared": "pink",
          "task-new": "green",
        },
        if_missing: true,
      },
    });
    await waitFor(() => expect(window.localStorage.getItem(TASK_COLORS_STORAGE_KEY)).toBeNull());
    expect(capturedStore?.getState().userSettings.sidebarTaskColors).toEqual({
      "task-existing": "red",
      "task-cleared": null,
      "task-new": "green",
    });
  });

  it("keeps the legacy key and does not use it for display when a batch fails", async () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, JSON.stringify({ "task-new": "green" }));
    mockUpdateUserSettings.mockRejectedValue(new Error("offline"));

    renderHook(() => useTaskColorMigration(true), { wrapper });

    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
    expect(window.localStorage.getItem(TASK_COLORS_STORAGE_KEY)).not.toBeNull();
    expect(capturedStore?.getState().userSettings.sidebarTaskColors).toEqual({
      "task-existing": "red",
      "task-cleared": null,
    });
  });

  it("does not start until the server settings state is loaded", async () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, JSON.stringify({ "task-new": "green" }));
    mockUpdateUserSettings.mockResolvedValue(response({ "task-new": "green" }, 2));

    const { rerender } = renderHook(() => useTaskColorMigration(true), {
      wrapper: ({ children }) => (
        <StateProvider
          initialState={{
            userSettings: { ...defaultState.userSettings, loaded: false },
          }}
        >
          {children}
        </StateProvider>
      ),
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();

    act(() => {
      rerender();
    });
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });
});
