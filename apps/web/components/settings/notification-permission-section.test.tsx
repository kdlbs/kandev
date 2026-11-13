import { cleanup, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import type { ComponentProps } from "react";
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
const ROW_SELECTOR = '[data-notification-preference-row="true"]';
const WARNING_SELECTOR = '[data-testid="notification-permission-warning"]';

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

type PermissionState = ComponentProps<typeof DesktopNotificationsSection>["notificationPermission"];

function section(permission: PermissionState) {
  return (
    <DesktopNotificationsSection
      notificationPermission={permission}
      onRequestPermission={vi.fn()}
      onRefreshPermission={vi.fn()}
      onTestNotification={vi.fn()}
    />
  );
}

function renderSection(permission: PermissionState) {
  return render(section(permission));
}

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
    renderSection("default");

    expect(screen.getByRole("button", { name: "Refresh notification permission" })).toBeTruthy();
  });

  it.each([
    ["default" as const, "Enable"],
    ["granted" as const, "Enabled"],
    ["error" as const, "Retry"],
  ])("labels the permission button %s as %s", (permission, label) => {
    renderSection(permission);

    expect(screen.getByRole("button", { name: label })).toBeTruthy();
  });
});

describe("DesktopNotificationsSection compact row", () => {
  it.each(["default", "granted"] as const)(
    "keeps the actions in one compact row without a reserved warning when permission is %s",
    (permission) => {
      mocks.nativeAvailable.mockReturnValue(false);
      const { container } = renderSection(permission);
      const row = container.querySelector(ROW_SELECTOR);

      expect(row).toBeTruthy();
      expect(row?.querySelector(WARNING_SELECTOR)).toBeNull();
      expect(container.querySelector('[data-settings-group="true"]')).toBeNull();
      for (const button of within(row as HTMLElement).getAllByRole("button")) {
        expect(row?.contains(button)).toBe(true);
      }
    },
  );

  it.each([
    { permission: "denied" as const, native: true, message: /OS app notification settings/i },
    { permission: "denied" as const, native: false, message: /site settings/i },
    {
      permission: "unsupported" as const,
      native: false,
      message: /does not support desktop notifications/i,
    },
    {
      permission: "error" as const,
      native: false,
      message: /could not check notification permission/i,
    },
  ])(
    "shows the corresponding warning inside the row for denied, unsupported, and error states ($permission, native $native)",
    ({ permission, native, message }) => {
      mocks.nativeAvailable.mockReturnValue(native);
      const { container } = renderSection(permission);
      const row = container.querySelector(ROW_SELECTOR);
      const warning = within(container).getByText(message);

      expect(row?.contains(warning)).toBe(true);
    },
  );

  it("updates the warning inside the same row when permission changes", () => {
    mocks.nativeAvailable.mockReturnValue(true);
    const { container, rerender } = renderSection("granted");
    const row = () => container.querySelector(ROW_SELECTOR);
    const warning = () => container.querySelector(WARNING_SELECTOR);

    expect(row()).toBeTruthy();
    expect(warning()).toBeNull();

    rerender(section("denied"));
    expect(warning()?.textContent).toMatch(/OS app notification settings/i);
    expect(row()?.contains(warning())).toBe(true);

    rerender(section("granted"));
    expect(warning()).toBeNull();
    expect(row()).toBeTruthy();
  });
});
