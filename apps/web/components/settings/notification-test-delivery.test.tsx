import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  DesktopNotificationsSection,
  useNotificationPermission,
} from "./notification-permission-section";
import { useNotificationsActions, useNotificationsState } from "./notifications-settings-actions";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import type {
  NativeNotificationPermission,
  NativeNotificationRequest,
  NativeNotificationResult,
} from "@/lib/desktop/native-notification-client";

const io = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  testProvider: vi.fn(),
  setProviders: vi.fn(),
  providers: [],
  events: [],
}));
vi.mock("@/lib/api", () => ({
  createNotificationProvider: io.create,
  updateNotificationProvider: io.update,
  deleteNotificationProvider: io.remove,
  testNotificationProvider: io.testProvider,
}));
vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: io.providers,
    events: io.events,
    appriseAvailable: false,
    loaded: true,
    loading: false,
  }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ setNotificationProviders: io.setProviders }),
}));

const BODY = "If you can read this, notifications are working on this device.";
let nativePermission: NativeNotificationPermission;
let showResult: NativeNotificationResult;
let browserPermission: NotificationPermission;
let requestedPermission: NativeNotificationPermission;
let failingCommand: string | undefined;
let denyOnShow: boolean;
const browserShow = vi.fn();
const browserRequest = vi.fn();
const invoke = vi.fn(async (command: string, _args?: Record<string, unknown>) => {
  if (command === "get_native_notification_permission") return nativePermission;
  if (command === "request_native_notification_permission") return nativePermission;
  if (command === "show_native_notification") return showResult;
  throw new Error(`Unexpected native command: ${command}`);
});
type NativeWindow = Window & { __TAURI_INTERNALS__?: { invoke: typeof invoke } };

function Consumer() {
  const state = useNotificationsState();
  const permission = useNotificationPermission();
  const actions = useNotificationsActions(state, permission.refreshPermission);
  return (
    <DesktopNotificationsSection
      notificationPermission={permission.notificationPermission}
      onRequestPermission={actions.handleRequestPermission}
      onRefreshPermission={actions.handleRefreshPermission}
      onTestNotification={actions.handleTestNotification}
    />
  );
}
function enableNative() {
  (window as NativeWindow).__TAURI_INTERNALS__ = { invoke };
}
function enableBrowser() {
  class BrowserNotification {
    static get permission() {
      return browserPermission;
    }
    static requestPermission = browserRequest;
    constructor(title: string, options: NotificationOptions) {
      browserShow(title, options);
    }
  }
  vi.stubGlobal("Notification", BrowserNotification);
}
async function clickTest() {
  await act(async () => {
    fireEvent.click(screen.getByTitle("Send test notification"));
  });
}
function nativeRequests(): NativeNotificationRequest[] {
  return invoke.mock.calls
    .filter(([command]) => command === "show_native_notification")
    .map(([, args]) => args?.request as NativeNotificationRequest);
}
beforeEach(() => {
  vi.clearAllMocks();
  invoke.mockReset();
  invoke.mockImplementation(async (command) => {
    if (command === failingCommand) throw new Error("OS command unavailable");
    if (command === "get_native_notification_permission") return nativePermission;
    if (command === "request_native_notification_permission") {
      nativePermission = requestedPermission;
      return nativePermission;
    }
    if (command === "show_native_notification") {
      if (denyOnShow) nativePermission = "denied";
      return showResult;
    }
    throw new Error(`Unexpected native command: ${command}`);
  });
  browserRequest.mockReset();
  browserRequest.mockResolvedValue("granted");
  requestedPermission = "granted";
  failingCommand = undefined;
  denyOnShow = false;
  nativePermission = "granted";
  showResult = "shown";
  browserPermission = "granted";
  vi.stubGlobal("Notification", undefined);
});
afterEach(() => {
  cleanup();
  delete (window as NativeWindow).__TAURI_INTERNALS__;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// @covers AC-DESKTOP-DESKTOP-TAURI-APP-001.15
// @covers AC-DESKTOP-DESKTOP-TAURI-APP-001.17
describe("settings Test delivery through the real consumer", () => {
  it("native granted with browser unavailable sends the device diagnostic", async () => {
    enableNative();
    render(<Consumer />);
    await waitFor(() => {
      expect((screen.getByRole("button", { name: "Enabled" }) as HTMLButtonElement).disabled).toBe(
        true,
      );
    });
    await clickTest();
    expect(nativeRequests()).toEqual([
      {
        eventId: expect.stringMatching(/^desktop\.test_notification:.+$/),
        title: "Test notification",
        body: BODY,
      },
    ]);
    expect(browserShow).not.toHaveBeenCalled();
    expect(io.create).not.toHaveBeenCalled();
    expect(io.update).not.toHaveBeenCalled();
    expect(io.remove).not.toHaveBeenCalled();
    expect(io.setProviders).not.toHaveBeenCalled();
  });
  it("browser granted uses the device-neutral diagnostic", async () => {
    enableBrowser();
    render(<Consumer />);
    await clickTest();
    expect(browserShow).toHaveBeenCalledWith("Test notification", { body: BODY });
    expect(invoke).not.toHaveBeenCalled();
  });
  it("the real native client and transport deliver through the OS invoke boundary", async () => {
    enableNative();
    const request = { eventId: "system.update_available:v1", title: "Control", body: "Control" };
    await expect(nativeNotifications.show(request)).resolves.toBe("shown");
    expect(invoke).toHaveBeenCalledWith("show_native_notification", { request });
  });
});

// @covers AC-DESKTOP-DESKTOP-TAURI-APP-001.16
describe("native Test permission and result handling", () => {
  it("native granted sends even when browser permission is denied", async () => {
    enableNative();
    browserPermission = "denied";
    enableBrowser();
    render(<Consumer />);

    await clickTest();

    expect(nativeRequests()).toHaveLength(1);
    expect(nativeRequests()[0]?.body).toBe(BODY);
    expect(browserRequest).not.toHaveBeenCalled();
    expect(browserShow).not.toHaveBeenCalled();
  });

  it.each(["prompt", "prompt-with-rationale"] as const)(
    "%s permission is requested by Test before sending",
    async (permission) => {
      enableNative();
      nativePermission = permission;
      render(<Consumer />);
      await waitFor(() => expect(screen.getByRole("button", { name: "Enable" })).toBeTruthy());
      expect(invoke.mock.calls.map(([command]) => command)).not.toContain(
        "request_native_notification_permission",
      );

      await clickTest();

      expect(nativeRequests()).toHaveLength(1);
      expect(screen.getByRole("button", { name: "Enabled" })).toBeTruthy();
      const commands = invoke.mock.calls.map(([command]) => command);
      expect(commands.indexOf("request_native_notification_permission")).toBeLessThan(
        commands.indexOf("show_native_notification"),
      );
    },
  );

  it("a denied prompt refreshes OS guidance without sending", async () => {
    enableNative();
    nativePermission = "prompt";
    requestedPermission = "denied";
    render(<Consumer />);

    await clickTest();

    expect(nativeRequests()).toEqual([]);
    expect(screen.getByText(/OS app notification settings/i)).toBeTruthy();
  });

  it("already denied permission never requests again or sends", async () => {
    enableNative();
    nativePermission = "denied";
    render(<Consumer />);
    await waitFor(() => expect(screen.getByText(/OS app notification settings/i)).toBeTruthy());

    await clickTest();
    await clickTest();

    expect(nativeRequests()).toEqual([]);
    expect(invoke.mock.calls.map(([command]) => command)).not.toContain(
      "request_native_notification_permission",
    );
  });

  it.each(["get_native_notification_permission", "request_native_notification_permission"])(
    "contains %s rejection and renders permission feedback",
    async (command) => {
      enableNative();
      nativePermission = "prompt";
      render(<Consumer />);
      await act(async () => {});
      failingCommand = command;

      await clickTest();

      expect(nativeRequests()).toEqual([]);
      expect(screen.getByText(/could not check notification permission/i)).toBeTruthy();
    },
  );
});

describe("native Test delivery results and occurrence identity", () => {
  it("show permission-denied refreshes actual OS permission without prompting", async () => {
    enableNative();
    showResult = "permission-denied";
    denyOnShow = true;
    render(<Consumer />);

    await clickTest();

    expect(nativeRequests()).toHaveLength(1);
    expect(screen.getByText(/OS app notification settings/i)).toBeTruthy();
    expect(invoke.mock.calls.map(([command]) => command)).not.toContain(
      "request_native_notification_permission",
    );
  });

  it.each(["display-failed", "rejection"])(
    "%s is contained without falsifying granted permission or falling back",
    async (failure) => {
      enableNative();
      enableBrowser();
      const diagnostic = vi.spyOn(console, "error").mockImplementation(() => {});
      if (failure === "rejection") failingCommand = "show_native_notification";
      else showResult = "display-failed";
      render(<Consumer />);

      await clickTest();

      expect(nativeRequests()).toHaveLength(1);
      expect(diagnostic).toHaveBeenCalledOnce();
      expect(screen.getByRole("button", { name: "Enabled" })).toBeTruthy();
      expect(screen.queryByText(/could not check notification permission/i)).toBeNull();
      expect(browserShow).not.toHaveBeenCalled();
      expect(browserRequest).not.toHaveBeenCalled();
    },
  );

  it("duplicate does not retry delivery or request permission", async () => {
    enableNative();
    showResult = "duplicate";
    render(<Consumer />);

    await clickTest();

    expect(nativeRequests()).toHaveLength(1);
    expect(invoke.mock.calls.map(([command]) => command)).not.toContain(
      "request_native_notification_permission",
    );
    expect(screen.getByRole("button", { name: "Enabled" })).toBeTruthy();
  });

  it("repeated clicks create distinct app-scoped occurrences", async () => {
    enableNative();
    render(<Consumer />);

    await clickTest();
    await clickTest();

    const requests = nativeRequests();
    expect(requests).toHaveLength(2);
    expect(new Set(requests.map((request) => request.eventId)).size).toBe(2);
    for (const request of requests) {
      expect(request.eventId).toMatch(/^desktop\.test_notification:.+$/);
      expect(request.eventId.length).toBeLessThanOrEqual(256);
      expect(request.taskId).toBeUndefined();
      expect(request.sessionId).toBeUndefined();
    }
  });
});

describe("browser-only Test behavior", () => {
  it.each(["granted", "denied"] as const)(
    "default permission prompts once and honors %s result",
    async (permission) => {
      browserPermission = "default";
      browserRequest.mockImplementation(async () => {
        browserPermission = permission;
        return permission;
      });
      enableBrowser();
      render(<Consumer />);

      await clickTest();

      expect(browserRequest).toHaveBeenCalledOnce();
      expect(browserShow).toHaveBeenCalledTimes(permission === "granted" ? 1 : 0);
      if (permission === "granted") {
        expect(browserShow).toHaveBeenCalledWith("Test notification", { body: BODY });
      } else {
        expect(screen.getByText(/site settings/i)).toBeTruthy();
      }
      expect(invoke).not.toHaveBeenCalled();
    },
  );

  it("denied permission does not prompt or send", async () => {
    browserPermission = "denied";
    enableBrowser();
    render(<Consumer />);

    await clickTest();

    expect(browserRequest).not.toHaveBeenCalled();
    expect(browserShow).not.toHaveBeenCalled();
  });

  it("unavailable browser notifications retain unsupported state", async () => {
    render(<Consumer />);
    await waitFor(() =>
      expect(screen.getByText(/does not support desktop notifications/i)).toBeTruthy(),
    );

    await clickTest();

    expect(browserShow).not.toHaveBeenCalled();
    expect(invoke).not.toHaveBeenCalled();
  });

  it("browser request rejection uses existing permission error feedback", async () => {
    browserPermission = "default";
    browserRequest.mockRejectedValue(new Error("Browser permission unavailable"));
    enableBrowser();
    render(<Consumer />);

    await clickTest();

    expect(browserShow).not.toHaveBeenCalled();
    expect(screen.getByText(/could not check notification permission/i)).toBeTruthy();
  });

  it("browser display rejection is contained without a false permission error", async () => {
    browserShow.mockImplementationOnce(() => {
      throw new Error("Browser display unavailable");
    });
    const diagnostic = vi.spyOn(console, "error").mockImplementation(() => {});
    enableBrowser();
    render(<Consumer />);

    await clickTest();

    expect(diagnostic).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Enabled" })).toBeTruthy();
    expect(screen.queryByText(/could not check notification permission/i)).toBeNull();
  });
});
