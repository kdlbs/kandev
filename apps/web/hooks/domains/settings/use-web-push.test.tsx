import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { useWebPush } from "./use-web-push";

const mocks = vi.hoisted(() => ({
  isPushSupported: vi.fn(),
  getPushRegistration: vi.fn(),
  getPushPublicKey: vi.fn(),
  listPushSubscriptions: vi.fn(),
  savePushSubscription: vi.fn(),
  deletePushSubscription: vi.fn(),
  sendTestPush: vi.fn(),
  listNotificationProviders: vi.fn(),
  setNotificationProviders: vi.fn(),
}));

vi.mock("@/lib/api/domains/push-api", () => ({
  getPushPublicKey: mocks.getPushPublicKey,
  listPushSubscriptions: mocks.listPushSubscriptions,
  savePushSubscription: mocks.savePushSubscription,
  deletePushSubscription: mocks.deletePushSubscription,
  sendTestPush: mocks.sendTestPush,
}));

vi.mock("@/lib/api", () => ({ listNotificationProviders: mocks.listNotificationProviders }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      setNotificationProviders: typeof mocks.setNotificationProviders;
    }) => unknown,
  ) => selector({ setNotificationProviders: mocks.setNotificationProviders }),
}));

vi.mock("@/lib/pwa/push-client", async () => {
  const actual =
    await vi.importActual<typeof import("@/lib/pwa/push-client")>("@/lib/pwa/push-client");
  return {
    ...actual,
    isPushSupported: mocks.isPushSupported,
    getPushRegistration: mocks.getPushRegistration,
  };
});

const ENDPOINT = "https://push.example/device";

function fakeSubscription() {
  return {
    endpoint: ENDPOINT,
    toJSON: () => ({ keys: { p256dh: "p", auth: "a" } }),
    unsubscribe: vi.fn().mockResolvedValue(true),
  };
}

function installNotification(permission: NotificationPermission, requested = permission) {
  const requestPermission = vi.fn().mockResolvedValue(requested);
  vi.stubGlobal("Notification", { permission, requestPermission });
  return requestPermission;
}

function setupRegistration(existing: ReturnType<typeof fakeSubscription> | null) {
  const pushManager = {
    getSubscription: vi.fn().mockResolvedValue(existing),
    subscribe: vi.fn().mockResolvedValue(fakeSubscription()),
  };
  mocks.getPushRegistration.mockResolvedValue({ pushManager });
  return pushManager;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  mocks.isPushSupported.mockReturnValue(true);
  mocks.getPushPublicKey.mockResolvedValue({ public_key: "BAAA" });
  mocks.listPushSubscriptions.mockResolvedValue({ subscriptions: [] });
  mocks.savePushSubscription.mockResolvedValue({ success: true });
  mocks.deletePushSubscription.mockResolvedValue({ success: true });
  mocks.listNotificationProviders.mockResolvedValue({
    providers: [{ id: "webpush", type: "web_push" }],
    events: ["session.failed"],
    apprise_available: true,
  });
});

describe("useWebPush initial state", () => {
  it("reports unsupported without touching the permission or worker", async () => {
    mocks.isPushSupported.mockReturnValue(false);
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("unsupported"));
    expect(mocks.getPushRegistration).not.toHaveBeenCalled();
  });

  it("reports denied when the browser blocked notifications", async () => {
    installNotification("denied");
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("denied"));
  });

  it("never prompts for permission on load", async () => {
    const request = installNotification("default");
    setupRegistration(null);
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("unsubscribed"));
    expect(request).not.toHaveBeenCalled();
  });

  it("reports subscribed when the server knows this device", async () => {
    installNotification("granted");
    setupRegistration(fakeSubscription());
    mocks.listPushSubscriptions.mockResolvedValue({ subscriptions: [{ endpoint: ENDPOINT }] });
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("subscribed"));
    expect(mocks.savePushSubscription).not.toHaveBeenCalled();
  });

  it("re-registers a browser subscription the server lost", async () => {
    installNotification("granted");
    setupRegistration(fakeSubscription());
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("subscribed"));
    expect(mocks.savePushSubscription).toHaveBeenCalledWith({
      endpoint: ENDPOINT,
      keys: { p256dh: "p", auth: "a" },
    });
  });

  it("reports session-expired when the API answers 401", async () => {
    installNotification("granted");
    setupRegistration(fakeSubscription());
    mocks.listPushSubscriptions.mockRejectedValue(new ApiError("no", 401, null));
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("session-expired"));
  });
});

describe("useWebPush actions", () => {
  it("enable asks for permission, subscribes with the server key and saves", async () => {
    const request = installNotification("default", "granted");
    const pushManager = setupRegistration(null);
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("unsubscribed"));

    await act(async () => {
      await result.current.enable();
    });

    expect(request).toHaveBeenCalledTimes(1);
    expect(pushManager.subscribe).toHaveBeenCalledWith(
      expect.objectContaining({ userVisibleOnly: true }),
    );
    expect(mocks.savePushSubscription).toHaveBeenCalledTimes(1);
    expect(mocks.listNotificationProviders).toHaveBeenCalledWith({ cache: "no-store" });
    expect(mocks.setNotificationProviders).toHaveBeenCalledWith({
      items: [{ id: "webpush", type: "web_push" }],
      events: ["session.failed"],
      appriseAvailable: true,
      loaded: true,
      loading: false,
    });
    expect(result.current.status).toBe("subscribed");
  });

  it("enable stops at denied when the user refuses", async () => {
    installNotification("default", "denied");
    const pushManager = setupRegistration(null);
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("unsubscribed"));

    await act(async () => {
      await result.current.enable();
    });

    expect(result.current.status).toBe("denied");
    expect(pushManager.subscribe).not.toHaveBeenCalled();
  });

  it("enable rolls the browser subscription back when the server save fails", async () => {
    installNotification("default", "granted");
    const created = fakeSubscription();
    const pushManager = setupRegistration(null);
    pushManager.subscribe.mockResolvedValue(created);
    mocks.savePushSubscription.mockRejectedValue(new ApiError("boom", 500, null));
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("unsubscribed"));

    await act(async () => {
      await result.current.enable();
    });

    expect(created.unsubscribe).toHaveBeenCalled();
    expect(result.current.status).toBe("error");
  });

  it("disable unsubscribes the browser and deletes the server record", async () => {
    installNotification("granted");
    const existing = fakeSubscription();
    setupRegistration(existing);
    mocks.listPushSubscriptions.mockResolvedValue({ subscriptions: [{ endpoint: ENDPOINT }] });
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("subscribed"));

    await act(async () => {
      await result.current.disable();
    });

    expect(existing.unsubscribe).toHaveBeenCalled();
    expect(mocks.deletePushSubscription).toHaveBeenCalledWith(ENDPOINT);
    expect(result.current.status).toBe("unsubscribed");
  });

  it("sendTest maps a 404 to the no-subscription result", async () => {
    installNotification("granted");
    setupRegistration(fakeSubscription());
    mocks.listPushSubscriptions.mockResolvedValue({ subscriptions: [{ endpoint: ENDPOINT }] });
    mocks.sendTestPush.mockRejectedValue(new ApiError("none", 404, null));
    const { result } = renderHook(() => useWebPush());
    await waitFor(() => expect(result.current.status).toBe("subscribed"));

    await act(async () => {
      await result.current.sendTest();
    });

    expect(result.current.testResult).toBe("no-subscription");
  });
});
