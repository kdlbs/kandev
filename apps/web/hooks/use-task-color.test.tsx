import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { TASK_COLORS_STORAGE_KEY } from "@/lib/task-colors";
import type { UserSettingsResponse } from "@/lib/types/http";
import { useSetTaskColor, useSetTaskColors, useTaskColor } from "./use-task-color";

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

  it("keeps the first persisted color when a later queued edit fails without a WS echo", async () => {
    let resolveFirst: ((value: UserSettingsResponse) => void) | undefined;
    mockUpdateUserSettings.mockImplementation(() => {
      if (mockUpdateUserSettings.mock.calls.length === 1) {
        return new Promise<UserSettingsResponse>((resolve) => {
          resolveFirst = resolve;
        });
      }
      return Promise.reject(new Error("offline"));
    });
    const { result } = renderHook(
      () => ({ color: useTaskColor("task-1"), setColor: useSetTaskColor() }),
      { wrapper },
    );

    act(() => {
      result.current.setColor("task-1", "blue");
      result.current.setColor("task-1", "green");
    });
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));

    act(() => resolveFirst?.(response({ "task-1": "blue" }, 2)));
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.color).toBe("blue"));
  });
});

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.5
it("restores only failed IDs across independent color setters", async () => {
  let resolveFirst: ((value: UserSettingsResponse) => void) | undefined;
  mockUpdateUserSettings.mockImplementation(() => {
    if (mockUpdateUserSettings.mock.calls.length === 1) {
      return new Promise<UserSettingsResponse>((resolve) => {
        resolveFirst = resolve;
      });
    }
    return Promise.reject(new Error("offline"));
  });
  const { result } = renderHook(() => ({ first: useSetTaskColor(), second: useSetTaskColor() }), {
    wrapper,
  });
  act(() => {
    result.current.first("task-1", "blue");
    result.current.second("task-2", "pink");
  });
  await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalled());
  act(() => resolveFirst?.(response({ "task-1": "blue" }, 2)));
  await waitFor(() =>
    expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-2"] ?? null).toBeNull(),
  );
  expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-1"]).toBe("blue");
});

it("does not roll back another hook's pending optimistic color", async () => {
  mockUpdateUserSettings.mockImplementation((payload) => {
    if (payload.sidebar_task_color_patch.colors["task-1"])
      return Promise.reject(new Error("offline"));
    return new Promise(() => {});
  });
  const { result } = renderHook(
    () => ({ first: useSetTaskColor(), second: useSetTaskColor(), color: useTaskColor("task-1") }),
    { wrapper },
  );
  act(() => {
    result.current.first("task-1", "blue");
    result.current.second("task-2", "pink");
  });
  await waitFor(() => expect(result.current.color).toBe("red"));
  expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-2"]).toBe("pink");
});

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.2, AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.7
describe("bulk colors", () => {
  it("deduplicates and captures IDs in one patch, including clearing", async () => {
    mockUpdateUserSettings.mockResolvedValue(response({ "task-1": null, "task-2": null }, 2));
    const { result } = renderHook(() => useSetTaskColors(), { wrapper });
    const ids = ["task-1", "task-2", "task-1", ""];
    let saving: Promise<unknown>;
    act(() => {
      saving = result.current.setColors(ids, null);
      ids.push("unselected");
    });
    expect(result.current.isPending).toBe(true);
    await act(async () => {
      await saving;
    });
    expect(mockUpdateUserSettings).toHaveBeenCalledExactlyOnceWith({
      sidebar_task_color_patch: { colors: { "task-1": null, "task-2": null }, if_missing: false },
    });
    expect(result.current.isPending).toBe(false);
  });

  it("does nothing for an empty selection", async () => {
    const { result } = renderHook(() => useSetTaskColors(), { wrapper });
    await act(async () => {
      expect(await result.current.setColors([], "red")).toEqual({ saved: 0, total: 0 });
    });
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });

  it.each([500, 501])("saves %i tasks in bounded sequential patches", async (count) => {
    let colors = { "task-1": "red" };
    let revision = 1;
    mockUpdateUserSettings.mockImplementation(async (payload) => {
      colors = { ...colors, ...payload.sidebar_task_color_patch.colors };
      return response(colors as Parameters<typeof response>[0], ++revision);
    });
    const { result } = renderHook(() => useSetTaskColors(), { wrapper });
    await act(async () => {
      expect(
        await result.current.setColors(
          Array.from({ length: count }, (_, i) => `bulk-${i}`),
          "purple",
        ),
      ).toEqual({ saved: count, total: count });
    });
    expect(mockUpdateUserSettings).toHaveBeenCalledTimes(Math.ceil(count / 500));
    expect(
      Object.keys(mockUpdateUserSettings.mock.calls[0][0].sidebar_task_color_patch.colors),
    ).toHaveLength(500);
    expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-1"]).toBe("red");
  });

  it("retains confirmed chunks and restores unsaved entries after partial failure", async () => {
    const ids = Array.from({ length: 501 }, (_, i) => `bulk-${i}`);
    mockUpdateUserSettings
      .mockResolvedValueOnce(
        response(
          {
            "task-1": "red",
            ...Object.fromEntries(ids.slice(0, 500).map((id) => [id, "purple" as const])),
          },
          2,
        ),
      )
      .mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useSetTaskColors(), { wrapper });
    await act(async () => {
      expect(await result.current.setColors(ids, "purple")).toEqual({ saved: 500, total: 501 });
    });
    const colors = capturedStore?.getState().userSettings.sidebarTaskColors;
    expect(colors?.["bulk-499"]).toBe("purple");
    expect(colors?.["bulk-500"] ?? null).toBeNull();
    expect(result.current.isPending).toBe(false);
  });

  it("guards same-tick duplicate submits and preserves a later single-task edit", async () => {
    let resolve: ((value: UserSettingsResponse) => void) | undefined;
    mockUpdateUserSettings
      .mockReturnValueOnce(
        new Promise<UserSettingsResponse>((r) => {
          resolve = r;
        }),
      )
      .mockResolvedValue(response({ "task-1": "pink", "task-2": "blue" }, 3));
    const { result } = renderHook(() => ({ bulk: useSetTaskColors(), single: useSetTaskColor() }), {
      wrapper,
    });
    let saving: Promise<unknown>;
    act(() => {
      saving = result.current.bulk.setColors(["task-1", "task-2"], "blue");
      expect(result.current.bulk.setColors(["task-1"], "green")).toBe(saving);
      result.current.single("task-1", "pink");
    });
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
    await act(async () => {
      resolve?.(response({ "task-1": "blue", "task-2": "blue" }, 2));
      await saving;
    });
    await waitFor(() =>
      expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-1"]).toBe("pink"),
    );
    expect(mockUpdateUserSettings).toHaveBeenCalledTimes(2);
  });
});

it("does not overwrite a newer single edit with a later chunk of an older batch", async () => {
  const ids = Array.from({ length: 501 }, (_, i) => `bulk-${i}`);
  let release: (() => void) | undefined;
  const barrier = new Promise<void>((resolve) => {
    release = resolve;
  });
  let confirmed: Parameters<typeof response>[0] = { "task-1": "red" };
  let revision = 1;
  mockUpdateUserSettings.mockImplementation(async (payload) => {
    if (mockUpdateUserSettings.mock.calls.length === 1) await barrier;
    confirmed = { ...confirmed, ...payload.sidebar_task_color_patch.colors };
    return response(confirmed, ++revision);
  });
  const { result } = renderHook(() => ({ bulk: useSetTaskColors(), single: useSetTaskColor() }), {
    wrapper,
  });
  let saving: Promise<unknown>;
  act(() => {
    saving = result.current.bulk.setColors(ids, "blue");
  });
  await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
  act(() => {
    result.current.single("bulk-500", "pink");
  });
  await act(async () => {
    release?.();
    await saving;
  });
  await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(2));
  expect(confirmed["bulk-500"]).toBe("pink");
  expect(capturedStore?.getState().userSettings.sidebarTaskColors["bulk-500"]).toBe("pink");
});

it("keeps a queued newer edit optimistic when an older write emits a settings event", async () => {
  let resolveFirst: ((value: UserSettingsResponse) => void) | undefined;
  mockUpdateUserSettings.mockImplementation(() => {
    if (mockUpdateUserSettings.mock.calls.length === 1) {
      return new Promise<UserSettingsResponse>((resolve) => {
        resolveFirst = resolve;
      });
    }
    return Promise.resolve(response({ "task-1": "pink", "task-2": "blue" }, 3));
  });
  const { result } = renderHook(() => ({ bulk: useSetTaskColors(), single: useSetTaskColor() }), {
    wrapper,
  });

  act(() => {
    void result.current.bulk.setColors(["task-1", "task-2"], "blue");
    result.current.single("task-1", "pink");
  });
  await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));
  act(() => {
    capturedStore?.setState((state) => ({
      ...state,
      userSettings: {
        ...state.userSettings,
        revision: 2,
        sidebarTaskColors: { "task-1": "blue", "task-2": "blue" },
      },
    }));
  });

  expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-1"]).toBe("pink");
  act(() => resolveFirst?.(response({ "task-1": "blue", "task-2": "blue" }, 2)));
  await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(2));
  await waitFor(() =>
    expect(capturedStore?.getState().userSettings.sidebarTaskColors["task-1"]).toBe("pink"),
  );
});
