import { act, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NotificationProvider } from "@/lib/types/http";
import {
  useIsDirty,
  useNotificationsActions,
  useNotificationsState,
  useSaveRequest,
} from "./notifications-settings-actions";
import {
  DesktopNotificationsSection,
  useNotificationPermission,
} from "./notification-permission-section";

const mocks = vi.hoisted(() => ({
  createNotificationProvider: vi.fn(),
  deleteNotificationProvider: vi.fn(),
  testNotificationProvider: vi.fn(),
  updateNotificationProvider: vi.fn(),
  setNotificationProviders: vi.fn(),
  nativeAvailable: vi.fn(),
  nativePermissionGet: vi.fn(),
  nativePermissionRequest: vi.fn(),
}));
const PAGER_URL = "json://pager";
const CLARIFICATION_EVENT = "session.clarification_requested";
const SEMANTIC_NOTIFICATION_EVENTS = [CLARIFICATION_EVENT, "session.turn_finished"];

vi.mock("@/lib/api", () => ({
  createNotificationProvider: mocks.createNotificationProvider,
  deleteNotificationProvider: mocks.deleteNotificationProvider,
  testNotificationProvider: mocks.testNotificationProvider,
  updateNotificationProvider: mocks.updateNotificationProvider,
}));

const savedProvider: NotificationProvider = {
  id: "provider-1",
  name: "Saved provider",
  type: "apprise",
  config: { urls: ["json://saved"] },
  enabled: true,
  events: ["task.completed"],
  created_at: "",
  updated_at: "",
};

let notificationProviders = [savedProvider];
let notificationEvents = ["task.completed"];
let notificationProvidersLoaded = true;
const originalNotification = globalThis.Notification;

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: notificationProviders,
    events: notificationEvents,
    appriseAvailable: true,
    loaded: notificationProvidersLoaded,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ setNotificationProviders: mocks.setNotificationProviders }),
}));

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: mocks.nativeAvailable,
    permission: {
      get: mocks.nativePermissionGet,
      request: mocks.nativePermissionRequest,
    },
  },
}));

function useHarness() {
  const state = useNotificationsState();
  const saveRequest = useSaveRequest(state);
  const actions = useNotificationsActions(state, vi.fn());
  return { state, saveRequest, actions, isDirty: useIsDirty(state) };
}

const createdProvider: NotificationProvider = {
  ...savedProvider,
  id: "provider-2",
  name: "Pager",
  config: { urls: [PAGER_URL] },
};

beforeEach(() => {
  vi.clearAllMocks();
  notificationProviders = [savedProvider];
  notificationEvents = ["task.completed"];
  notificationProvidersLoaded = true;
  mocks.nativeAvailable.mockReturnValue(true);
  mocks.createNotificationProvider.mockResolvedValue(createdProvider);
});

afterEach(() => {
  Object.defineProperty(globalThis, "Notification", {
    configurable: true,
    value: originalNotification,
  });
});

describe("notification provider draft hydration", () => {
  it("hydrates a cold provider load into the draft", () => {
    notificationProviders = [];
    notificationEvents = [];
    notificationProvidersLoaded = false;
    const { result, rerender } = renderHook(useHarness);

    notificationProviders = [savedProvider];
    notificationEvents = SEMANTIC_NOTIFICATION_EVENTS;
    notificationProvidersLoaded = true;
    rerender();

    expect(result.current.state.providers).toEqual([savedProvider]);
    expect(result.current.state.baselineProviders).toEqual([savedProvider]);
    expect(result.current.state.notificationEvents).toEqual(SEMANTIC_NOTIFICATION_EVENTS);
  });

  it("hydrates the first provider load while retaining a typed create draft", async () => {
    notificationProviders = [];
    notificationEvents = [];
    notificationProvidersLoaded = false;
    const { result, rerender } = renderHook(useHarness);

    act(() => {
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseName("Pager");
      result.current.state.setAppriseUrls(PAGER_URL);
    });

    notificationProviders = [savedProvider];
    notificationEvents = SEMANTIC_NOTIFICATION_EVENTS;
    notificationProvidersLoaded = true;
    rerender();

    expect(result.current.state.providers).toEqual([savedProvider]);
    expect(result.current.state.baselineProviders).toEqual([savedProvider]);
    expect(result.current.state.appriseName).toBe("Pager");
    expect(result.current.state.appriseUrls).toBe(PAGER_URL);

    await act(() => result.current.saveRequest.run());

    expect(result.current.state.providers).toEqual([savedProvider, createdProvider]);
    expect(mocks.setNotificationProviders).toHaveBeenCalledWith(
      expect.objectContaining({ items: [savedProvider, createdProvider] }),
    );
  });

  it("does not replace an edited draft when providers refresh", () => {
    const { result, rerender } = renderHook(useHarness);
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, "Draft name"));

    const refreshedProvider = { ...savedProvider, name: "Server refresh" };
    notificationProviders = [refreshedProvider];
    rerender();

    expect(result.current.state.providers[0]?.name).toBe("Draft name");
    expect(result.current.state.baselineProviders[0]?.name).toBe(savedProvider.name);
  });

  it("hydrates a deferred provider refresh after discarding the draft", () => {
    const { result, rerender } = renderHook(useHarness);
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, "Draft name"));
    const refreshedProvider = { ...savedProvider, name: "Server refresh" };
    notificationProviders = [refreshedProvider];
    rerender();

    act(() => result.current.actions.discard());

    expect(result.current.state.providers).toEqual([refreshedProvider]);
    expect(result.current.state.baselineProviders).toEqual([refreshedProvider]);
  });
});

describe("notification provider draft saving", () => {
  it("stages a new Apprise provider until the shared save runs", async () => {
    const { result } = renderHook(useHarness);

    act(() => result.current.actions.openAppriseForm("create"));
    expect(result.current.isDirty).toBe(true);
    expect(mocks.createNotificationProvider).not.toHaveBeenCalled();

    act(() => {
      result.current.state.setAppriseName("Pager");
      result.current.state.setAppriseUrls(PAGER_URL);
    });
    await act(() => result.current.saveRequest.run());

    expect(mocks.createNotificationProvider).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Pager",
        config: { urls: [PAGER_URL] },
        events: [CLARIFICATION_EVENT],
      }),
    );
    expect(result.current.state.providers).toContainEqual(createdProvider);
    expect(result.current.state.showAppriseForm).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });

  it("keeps a failed create draft open and dirty", async () => {
    mocks.createNotificationProvider.mockRejectedValueOnce(new Error("create unavailable"));
    const { result } = renderHook(useHarness);
    act(() => {
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseName("Pager");
      result.current.state.setAppriseUrls(PAGER_URL);
    });

    await act(async () => {
      await expect(result.current.saveRequest.run()).rejects.toThrow("create unavailable");
    });

    expect(result.current.state.showAppriseForm).toBe(true);
    expect(result.current.state.appriseName).toBe("Pager");
    expect(result.current.state.appriseUrls).toBe(PAGER_URL);
    expect(result.current.isDirty).toBe(true);
  });

  it("cancels a new draft without persisting it", () => {
    const { result } = renderHook(useHarness);
    act(() => {
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseUrls(PAGER_URL);
    });

    act(() => result.current.actions.cancelAppriseForm());

    expect(mocks.createNotificationProvider).not.toHaveBeenCalled();
    expect(result.current.state.showAppriseForm).toBe(false);
    expect(result.current.state.appriseUrls).toBe("");
    expect(result.current.isDirty).toBe(false);
  });

  it("discards staged provider edits back to the loaded baseline", () => {
    const { result } = renderHook(useHarness);
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, "Changed"));
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.actions.discard());

    expect(result.current.state.providers).toEqual([savedProvider]);
    expect(result.current.isDirty).toBe(false);
  });
});

describe("notification permission actions", () => {
  it("surfaces a rejected native permission query as an actionable error state", async () => {
    mocks.nativePermissionGet.mockRejectedValueOnce(new Error("native permission unavailable"));

    const { result } = renderHook(() => useNotificationPermission());

    await waitFor(() => expect(result.current.notificationPermission).toBe("error"));
  });

  it("renders actionable feedback when the permission state cannot be queried", () => {
    render(
      <DesktopNotificationsSection
        notificationPermission="error"
        onRequestPermission={vi.fn()}
        onRefreshPermission={vi.fn()}
        onTestNotification={vi.fn()}
      />,
    );

    expect(screen.getByText(/could not check notification permission/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("requests native permission from the user action when Tauri is available", async () => {
    mocks.nativePermissionRequest.mockResolvedValue("granted");
    const refreshPermission = vi.fn();
    const { result } = renderHook(() => {
      const state = useNotificationsState();
      return useNotificationsActions(state, refreshPermission);
    });

    await act(() => result.current.handleRequestPermission());

    expect(mocks.nativePermissionRequest).toHaveBeenCalledOnce();
    expect(refreshPermission).toHaveBeenCalledOnce();
  });

  it("handles a rejected native permission request without leaking the rejection", async () => {
    mocks.nativePermissionRequest.mockRejectedValueOnce(new Error("native permission unavailable"));
    const refreshPermission = vi.fn();
    const { result } = renderHook(() => {
      const state = useNotificationsState();
      return useNotificationsActions(state, refreshPermission);
    });

    await expect(result.current.handleRequestPermission()).resolves.toBeUndefined();

    expect(refreshPermission).toHaveBeenCalledWith(expect.any(Error));
  });

  it("reports a rejected browser permission request without leaking the rejection", async () => {
    mocks.nativePermissionRequest.mockReset();
    const originalNotification = globalThis.Notification;
    Object.defineProperty(globalThis, "Notification", {
      configurable: true,
      value: {
        permission: "default",
        requestPermission: vi.fn().mockRejectedValue(new Error("blocked")),
      },
    });
    const refreshPermission = vi.fn();
    mocks.nativeAvailable.mockReturnValueOnce(false);
    const { result } = renderHook(() => {
      const state = useNotificationsState();
      return useNotificationsActions(state, refreshPermission);
    });

    await expect(result.current.handleRequestPermission()).resolves.toBeUndefined();

    expect(refreshPermission).toHaveBeenCalledWith(expect.any(Error));
    Object.defineProperty(globalThis, "Notification", {
      configurable: true,
      value: originalNotification,
    });
  });
});

describe("notification permission transport behavior", () => {
  it("requests browser permission alongside native permission for Office notification fallback", async () => {
    mocks.nativePermissionRequest.mockResolvedValue("granted");
    const requestPermission = vi.fn().mockResolvedValue("granted");
    Object.defineProperty(globalThis, "Notification", {
      configurable: true,
      value: { permission: "default", requestPermission },
    });
    const refreshPermission = vi.fn();
    const { result } = renderHook(() => {
      const state = useNotificationsState();
      return useNotificationsActions(state, refreshPermission);
    });

    await act(() => result.current.handleRequestPermission());

    expect(mocks.nativePermissionRequest).toHaveBeenCalledOnce();
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(refreshPermission).toHaveBeenCalledOnce();
  });

  it("directs denied native notifications to OS app settings", () => {
    mocks.nativeAvailable.mockReturnValue(true);
    const { container } = render(
      <DesktopNotificationsSection
        notificationPermission="denied"
        onRequestPermission={vi.fn()}
        onRefreshPermission={vi.fn()}
        onTestNotification={vi.fn()}
      />,
    );

    expect(within(container).getByText(/OS app notification settings/i)).toBeTruthy();
  });

  it("directs denied browser notifications to site settings", () => {
    mocks.nativeAvailable.mockReturnValue(false);
    const { container } = render(
      <DesktopNotificationsSection
        notificationPermission="denied"
        onRequestPermission={vi.fn()}
        onRefreshPermission={vi.fn()}
        onTestNotification={vi.fn()}
      />,
    );

    expect(within(container).getByText(/site settings/i)).toBeTruthy();
  });
});
