import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";
import type { useUpdateAvailableToast as UseUpdateAvailableToastHook } from "./use-update-available-toast";

let mockNotification: UpdateAvailableNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();
const mockNativeIsAvailable = vi.fn(() => false);
const mockNativeShow = vi.fn().mockResolvedValue("shown");
const UPDATE_VERSION = "v1.2.3";
const UPDATE_TITLE = "Kandev update available";

function updateNotification(): UpdateAvailableNotification {
  return {
    version: UPDATE_VERSION,
    title: UPDATE_TITLE,
    body: `Kandev ${UPDATE_VERSION} is available.`,
    occurrence_id: UPDATE_VERSION,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      updateAvailableNotification: mockNotification,
      setUpdateAvailableNotification: mockClearNotification,
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

let useUpdateAvailableToast: typeof UseUpdateAvailableToastHook;

describe("useUpdateAvailableToast", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    vi.resetModules();
    mockNativeIsAvailable.mockReturnValue(false);
    mockNotification = null;
    ({ useUpdateAvailableToast } = await import("./use-update-available-toast"));
  });

  it("does nothing when there is no notification", () => {
    renderHook(() => useUpdateAvailableToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("always shows an in-app toast for a Local update occurrence", () => {
    mockNotification = updateNotification();
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: UPDATE_TITLE,
        description: expect.stringContaining(UPDATE_VERSION),
      }),
    );
    expect(mockNativeShow).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("also attempts native delivery without replacing the toast", () => {
    mockNotification = updateNotification();
    mockNativeIsAvailable.mockReturnValue(true);
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockNativeShow).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId: `system.update_available:${UPDATE_VERSION}`,
        title: UPDATE_TITLE,
      }),
    );
  });

  it("retains the toast when native delivery is denied", () => {
    mockNotification = updateNotification();
    mockNativeIsAvailable.mockReturnValue(true);
    mockNativeShow.mockResolvedValueOnce("permission-denied");
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockNativeShow).toHaveBeenCalledTimes(1);
  });

  it("deduplicates repeated notifications for the same version", () => {
    mockNotification = updateNotification();
    const { rerender } = renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();
    mockNotification = updateNotification();
    rerender();
    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });
});
