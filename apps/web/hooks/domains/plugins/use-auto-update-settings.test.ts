import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

const { getPluginSettingsSpy, updatePluginSettingsSpy } = vi.hoisted(() => ({
  getPluginSettingsSpy: vi.fn(),
  updatePluginSettingsSpy: vi.fn(),
}));

vi.mock("@/lib/api/domains/plugins-api", () => ({
  getPluginSettings: (...args: unknown[]) => getPluginSettingsSpy(...args),
  updatePluginSettings: (...args: unknown[]) => updatePluginSettingsSpy(...args),
}));

import { useAutoUpdateSettings } from "./use-auto-update-settings";

beforeEach(() => {
  getPluginSettingsSpy.mockReset();
  updatePluginSettingsSpy.mockReset();
  (toast.error as ReturnType<typeof vi.fn>).mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useAutoUpdateSettings", () => {
  it("loads the instance-wide default on mount", async () => {
    getPluginSettingsSpy.mockResolvedValue({ auto_update_default: true });

    const { result } = renderHook(() => useAutoUpdateSettings());

    // Starts off, not yet loaded.
    expect(result.current.autoUpdateDefault).toBe(false);
    expect(result.current.loaded).toBe(false);

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.autoUpdateDefault).toBe(true);
  });

  it("marks loaded (default off) when the initial fetch fails, without throwing", async () => {
    getPluginSettingsSpy.mockRejectedValue(new Error("backend unreachable"));

    const { result } = renderHook(() => useAutoUpdateSettings());

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.autoUpdateDefault).toBe(false);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("optimistically applies setDefault and reconciles with the persisted value", async () => {
    getPluginSettingsSpy.mockResolvedValue({ auto_update_default: false });
    updatePluginSettingsSpy.mockResolvedValue({ auto_update_default: true });

    const { result } = renderHook(() => useAutoUpdateSettings());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.setDefault(true);
    });

    expect(updatePluginSettingsSpy).toHaveBeenCalledWith(true);
    expect(result.current.autoUpdateDefault).toBe(true);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("rolls back the optimistic update and toasts when the write fails", async () => {
    getPluginSettingsSpy.mockResolvedValue({ auto_update_default: false });
    updatePluginSettingsSpy.mockRejectedValue(new Error("write failed"));

    const { result } = renderHook(() => useAutoUpdateSettings());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.setDefault(true);
    });

    expect(result.current.autoUpdateDefault).toBe(false); // rolled back
    expect(toast.error).toHaveBeenCalledWith("write failed");
  });
});
