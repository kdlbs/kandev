import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";
import type { UpdateNotificationSettings } from "@/lib/types/system";

let mockNotification: UpdateAvailableNotification | null = null;
let mockSettings: UpdateNotificationSettings | null = null;
const mockClearNotification = vi.fn();
const mockSetSettings = vi.fn();
const mockToast = vi.fn();
const mockNativeIsAvailable = vi.fn(() => false);
const mockNativeShow = vi.fn().mockResolvedValue("shown");
const mockFetchSettings = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      updateAvailableNotification: mockNotification,
      setUpdateAvailableNotification: mockClearNotification,
      system: { updateNotificationSettings: mockSettings },
      setSystemUpdateNotificationSettings: mockSetSettings,
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: () => mockNativeIsAvailable(),
    show: (request: unknown) => mockNativeShow(request),
  },
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  fetchUpdateNotificationSettings: (...args: unknown[]) => mockFetchSettings(...args),
}));

import { useUpdateAvailableToast } from "./use-update-available-toast";

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("useUpdateAvailableToast", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNativeIsAvailable.mockReturnValue(false);
    mockNotification = null;
    mockSettings = null;
  });

  it("does nothing when there is no notification", () => {
    renderHook(() => useUpdateAvailableToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("shows an in-app toast when the channel is in_view", async () => {
    mockSettings = { enabled: true, channel: "in_view" };
    mockNotification = { version: "v1.2.3" };
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Update available",
        description: expect.stringContaining("v1.2.3"),
      }),
    );
    expect(mockNativeShow).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("shows a native notification (not a toast) when the channel is desktop and native is available", async () => {
    mockSettings = { enabled: true, channel: "desktop" };
    mockNotification = { version: "v1.2.3" };
    mockNativeIsAvailable.mockReturnValue(true);
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockNativeShow).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId: "system.update_available:v1.2.3",
        title: "Update available",
      }),
    );
  });

  it("shows both a toast and a native notification when the channel is both", async () => {
    mockSettings = { enabled: true, channel: "both" };
    mockNotification = { version: "v1.2.3" };
    mockNativeIsAvailable.mockReturnValue(true);
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();

    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockNativeShow).toHaveBeenCalledTimes(1);
  });

  it("does not notify on any channel when notifications are disabled", async () => {
    mockSettings = { enabled: false, channel: "both" };
    mockNotification = { version: "v1.2.3" };
    mockNativeIsAvailable.mockReturnValue(true);
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockNativeShow).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("fetches settings lazily when not yet loaded, then caches them in the store", async () => {
    mockSettings = null;
    mockFetchSettings.mockResolvedValue({ enabled: true, channel: "in_view" });
    mockNotification = { version: "v1.2.3" };
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();

    expect(mockFetchSettings).toHaveBeenCalledTimes(1);
    expect(mockSetSettings).toHaveBeenCalledWith({ enabled: true, channel: "in_view" });
    expect(mockToast).toHaveBeenCalledTimes(1);
  });

  it("falls back to both channels when the settings fetch fails", async () => {
    mockSettings = null;
    mockFetchSettings.mockRejectedValue(new Error("network error"));
    mockNotification = { version: "v1.2.3" };
    renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();
    await flushMicrotasks();

    expect(mockToast).toHaveBeenCalledTimes(1);
  });

  it("deduplicates repeated notifications for the same version", async () => {
    mockSettings = { enabled: true, channel: "in_view" };
    mockNotification = { version: "v1.2.3" };
    const { rerender } = renderHook(() => useUpdateAvailableToast());
    await flushMicrotasks();
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();
    mockNotification = { version: "v1.2.3" };
    rerender();
    await flushMicrotasks();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });
});
