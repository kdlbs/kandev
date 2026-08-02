import { cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  DesktopNotificationsSection,
  useNotificationPermission,
} from "./notification-permission-section";

const mocks = vi.hoisted(() => ({
  nativeAvailable: vi.fn(),
  nativePermissionGet: vi.fn(),
}));
const originalNotification = globalThis.Notification;

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: mocks.nativeAvailable,
    permission: { get: mocks.nativePermissionGet },
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
  Object.defineProperty(globalThis, "Notification", {
    configurable: true,
    value: originalNotification,
  });
});

describe("useNotificationPermission", () => {
  it("reads native notification permission when Tauri is available", async () => {
    mocks.nativeAvailable.mockReturnValue(true);
    mocks.nativePermissionGet.mockResolvedValue("granted");

    const { result } = renderHook(() => useNotificationPermission());

    await waitFor(() => expect(result.current.notificationPermission).toBe("granted"));
    expect(mocks.nativePermissionGet).toHaveBeenCalledOnce();
  });

  it("reads browser notification permission when Tauri is unavailable", async () => {
    mocks.nativeAvailable.mockReturnValue(false);
    Object.defineProperty(globalThis, "Notification", {
      configurable: true,
      value: { permission: "denied" },
    });

    const { result } = renderHook(() => useNotificationPermission());

    await waitFor(() => expect(result.current.notificationPermission).toBe("denied"));
  });

  it("reports unsupported when browser notifications are unavailable", async () => {
    mocks.nativeAvailable.mockReturnValue(false);
    Object.defineProperty(globalThis, "Notification", {
      configurable: true,
      value: undefined,
    });

    const { result } = renderHook(() => useNotificationPermission());

    await waitFor(() => expect(result.current.notificationPermission).toBe("unsupported"));
  });

  it("reports an error when the native permission query rejects", async () => {
    mocks.nativeAvailable.mockReturnValue(true);
    mocks.nativePermissionGet.mockRejectedValueOnce(new Error("native permission unavailable"));

    const { result } = renderHook(() => useNotificationPermission());

    await waitFor(() => expect(result.current.notificationPermission).toBe("error"));
  });
});

describe("DesktopNotificationsSection", () => {
  it("names the permission refresh button", () => {
    render(
      <DesktopNotificationsSection
        notificationPermission="default"
        onRequestPermission={vi.fn()}
        onRefreshPermission={vi.fn()}
        onTestNotification={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Refresh notification permission" })).toBeTruthy();
  });

  it.each([
    ["default" as const, "Enable"],
    ["granted" as const, "Enabled"],
    ["error" as const, "Retry"],
  ])("labels the permission button %s as %s", (permission, label) => {
    render(
      <DesktopNotificationsSection
        notificationPermission={permission}
        onRequestPermission={vi.fn()}
        onRefreshPermission={vi.fn()}
        onTestNotification={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: label })).toBeTruthy();
  });
});
