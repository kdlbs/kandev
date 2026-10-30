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
const GROUP_SELECTOR = '[data-settings-group="true"]';
const GROUP_CARD_SELECTOR = '[data-settings-group-card="true"]';
const GROUP_CONTENT_SELECTOR = '[data-settings-group-content="true"]';
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

describe("DesktopNotificationsSection permission frame", () => {
  it.each(["default", "granted"] as const)(
    "omits the warning card when permission is %s",
    (permission) => {
      mocks.nativeAvailable.mockReturnValue(false);
      const { container } = renderSection(permission);
      const group = container.querySelector(GROUP_SELECTOR);
      const content = group?.querySelector(GROUP_CONTENT_SELECTOR);

      expect(group).toBeTruthy();
      expect(group?.querySelector(GROUP_CARD_SELECTOR)).toBeNull();
      expect(group?.querySelector(WARNING_SELECTOR)).toBeNull();
      expect(content?.childElementCount).toBe(0);
      expect(content?.classList.contains("hidden")).toBe(true);
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
    "shows the corresponding warning for denied, unsupported, and error states ($permission, native $native)",
    ({ permission, native, message }) => {
      mocks.nativeAvailable.mockReturnValue(native);
      const { container } = renderSection(permission);
      const card = container.querySelector(GROUP_CARD_SELECTOR);
      const warning = within(container).getByText(message);

      expect(card).toBeTruthy();
      expect(card?.contains(warning)).toBe(true);
      expect(container.querySelector(GROUP_CONTENT_SELECTOR)).toBeNull();
    },
  );

  it("updates the warning frame when permission changes", () => {
    mocks.nativeAvailable.mockReturnValue(true);
    const { container, rerender } = renderSection("granted");
    const group = () => container.querySelector(GROUP_SELECTOR);
    const warning = () => container.querySelector(WARNING_SELECTOR);

    expect(group()?.querySelector(GROUP_CARD_SELECTOR)).toBeNull();

    rerender(section("denied"));
    expect(group()?.querySelector(GROUP_CARD_SELECTOR)).toBeTruthy();
    expect(warning()?.textContent).toMatch(/OS app notification settings/i);

    rerender(section("granted"));
    expect(group()?.querySelector(GROUP_CARD_SELECTOR)).toBeNull();
    expect(warning()).toBeNull();
    expect(group()?.querySelector(GROUP_CONTENT_SELECTOR)?.classList.contains("hidden")).toBe(true);
  });
});
