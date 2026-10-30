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
  formatWebhookHeaders,
  parseWebhookHeaders,
  webhookHeadersError,
} from "./notifications-settings-webhook-helpers";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
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
const SAVED_EVENT = "task.completed";
const DRAFT_NAME = "Draft name";
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
  events: [SAVED_EVENT],
  created_at: "",
  updated_at: "",
};
const localProvider: NotificationProvider = {
  ...savedProvider,
  id: "provider-local",
  name: "Local provider",
  type: "local",
  config: {},
};

let notificationProviders = [savedProvider];
let notificationEvents = [SAVED_EVENT];
let notificationProvidersLoaded = true;
let notificationAppriseAvailable = true;
const originalNotification = globalThis.Notification;

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: notificationProviders,
    events: notificationEvents,
    appriseAvailable: notificationAppriseAvailable,
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
  notificationEvents = [SAVED_EVENT];
  notificationProvidersLoaded = true;
  notificationAppriseAvailable = true;
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
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, DRAFT_NAME));

    const refreshedProvider = { ...savedProvider, name: "Server refresh" };
    notificationProviders = [refreshedProvider];
    rerender();

    expect(result.current.state.providers[0]?.name).toBe(DRAFT_NAME);
    expect(result.current.state.baselineProviders[0]?.name).toBe(savedProvider.name);
  });

  it("hydrates a deferred provider refresh after discarding the draft", () => {
    const { result, rerender } = renderHook(useHarness);
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, DRAFT_NAME));
    const refreshedProvider = { ...savedProvider, name: "Server refresh" };
    notificationProviders = [refreshedProvider];
    rerender();

    act(() => result.current.actions.discard());

    expect(result.current.state.providers).toEqual([refreshedProvider]);
    expect(result.current.state.baselineProviders).toEqual([refreshedProvider]);
  });

  it("updates availability without replacing dirty drafts or pending removals", () => {
    notificationProviders = [savedProvider, localProvider];
    const { result, rerender } = renderHook(useHarness);

    act(() => {
      result.current.actions.handleAppriseNameEdit(savedProvider.id, DRAFT_NAME);
      result.current.actions.handleAppriseEdit(savedProvider.id, PAGER_URL);
      result.current.actions.handleToggleEvent(savedProvider, CLARIFICATION_EVENT);
      result.current.actions.handleDeleteProvider(localProvider.id);
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseName("New provider");
      result.current.state.setAppriseUrls("json://new");
    });

    notificationAppriseAvailable = false;
    rerender();

    expect(result.current.state.appriseAvailable).toBe(false);
    expect(result.current.state.providers).toEqual([
      expect.objectContaining({
        id: savedProvider.id,
        name: DRAFT_NAME,
        config: { urls: [PAGER_URL] },
        events: [SAVED_EVENT, CLARIFICATION_EVENT],
      }),
    ]);
    expect(result.current.state.pendingDeletes).toEqual(new Set([localProvider.id]));
    expect(result.current.state.appriseName).toBe("New provider");
    expect(result.current.state.appriseUrls).toBe("json://new");
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

  it("does not overwrite a rescan that completes during a provider save", async () => {
    let resolveUpdate: (provider: NotificationProvider) => void = () => undefined;
    const updatePromise = new Promise<NotificationProvider>((resolve) => {
      resolveUpdate = resolve;
    });
    mocks.updateNotificationProvider.mockReturnValueOnce(updatePromise);
    const { result, rerender } = renderHook(useHarness);

    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, DRAFT_NAME));
    let savePromise: Promise<unknown>;
    act(() => {
      savePromise = result.current.saveRequest.run();
    });
    await waitFor(() => expect(mocks.updateNotificationProvider).toHaveBeenCalledOnce());

    notificationAppriseAvailable = false;
    rerender();

    await act(async () => {
      resolveUpdate({ ...savedProvider, name: DRAFT_NAME });
      await savePromise;
    });

    const lastSnapshot = mocks.setNotificationProviders.mock.lastCall?.[0];
    expect(lastSnapshot).toEqual({
      items: [{ ...savedProvider, name: DRAFT_NAME }],
      events: [SAVED_EVENT],
      loaded: true,
      loading: false,
    });
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

const INVALID_WEBHOOK_HEADERS = "settings:webhookHeadersInvalid";

const webhookProvider: NotificationProvider = {
  id: "provider-webhook",
  name: "GoClaw",
  type: "webhook",
  config: {
    url: "https://goclaw.example/hooks/kandev/turn-finished",
    headers: { "X-Tenant": "acme" },
    secret: "gateway-token",
  },
  enabled: true,
  events: [SAVED_EVENT],
  created_at: "",
  updated_at: "",
};

const GOCLAW_CLARIFICATION_URL = "https://goclaw.example/hooks/kandev/clarification";
const WEBHOOK_TEST_NAME = "Inbound hooks";

describe("webhook provider drafts", () => {
  it("parses, formats, and validates webhook headers", () => {
    expect(parseWebhookHeaders("X-Tenant: acme\nAuthorization: Bearer abc\n\ninvalid")).toEqual({
      "X-Tenant": "acme",
      Authorization: "Bearer abc",
    });
    expect(formatWebhookHeaders({ "X-Tenant": "acme", Authorization: "Bearer abc" })).toBe(
      "X-Tenant: acme\nAuthorization: Bearer abc",
    );
    expect(webhookHeadersError("X-Tenant: acme\nAuthorization: Bearer abc")).toBeNull();
    expect(webhookHeadersError("\n  \nX-Tenant: acme\n")).toBeNull();
    expect(webhookHeadersError("X-Trace: a:b:c")).toBeNull();
    expect(webhookHeadersError("X-Trace: a\tb")).toBeNull();
    expect(webhookHeadersError("X Tenant: acme")).toBe(INVALID_WEBHOOK_HEADERS);
    expect(webhookHeadersError("X@Tenant: acme")).toBe(INVALID_WEBHOOK_HEADERS);
    expect(webhookHeadersError("X-Tenant acme")).toBe(INVALID_WEBHOOK_HEADERS);
    expect(webhookHeadersError("X-Tenant: acme\nx-tenant: other")).toBe(INVALID_WEBHOOK_HEADERS);
    expect(webhookHeadersError(" X-Tenant : acme\nx-tenant: other")).toBe(INVALID_WEBHOOK_HEADERS);
    expect(webhookHeadersError("X-Tenant: ac\rme")).toBe(INVALID_WEBHOOK_HEADERS);
  });

  it("stages a new webhook provider with url, headers, and secret", async () => {
    const createdWebhook: NotificationProvider = {
      ...webhookProvider,
      id: "provider-webhook-new",
      name: WEBHOOK_TEST_NAME,
    };
    mocks.createNotificationProvider.mockResolvedValueOnce(createdWebhook);
    const { result } = renderHook(useHarness);

    act(() => result.current.actions.openWebhookForm("create"));
    expect(result.current.isDirty).toBe(true);

    act(() => {
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl("http://goclaw.local:18789/hooks/kandev/turn-finished");
      result.current.state.setWebhookHeaders("X-Tenant: acme");
      result.current.state.setWebhookSecret("gateway-token");
    });
    await act(() => result.current.saveRequest.run());

    expect(mocks.createNotificationProvider).toHaveBeenCalledWith({
      name: WEBHOOK_TEST_NAME,
      type: "webhook",
      config: {
        url: "http://goclaw.local:18789/hooks/kandev/turn-finished",
        headers: { "X-Tenant": "acme" },
        secret: "gateway-token",
      },
      enabled: true,
      events: DEFAULT_NOTIFICATION_EVENTS,
    });
    expect(result.current.state.providers).toContainEqual(createdWebhook);
    expect(result.current.state.showWebhookForm).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });

  it("refuses to save a webhook create draft without a url", async () => {
    const { result } = renderHook(useHarness);
    act(() => result.current.actions.openWebhookForm("create"));

    await act(async () => {
      await expect(result.current.saveRequest.run()).rejects.toThrow("A webhook URL is required.");
    });

    expect(mocks.createNotificationProvider).not.toHaveBeenCalled();
    expect(result.current.state.showWebhookForm).toBe(true);
    expect(result.current.isDirty).toBe(true);
  });

  it("updates webhook config fields and restores them on cancel", async () => {
    notificationProviders = [webhookProvider];
    const { result } = renderHook(useHarness);

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, {
        url: GOCLAW_CLARIFICATION_URL,
      });
      result.current.actions.handleWebhookEdit(webhookProvider.id, { headers: "X-Tenant: beta" });
      result.current.actions.handleWebhookEdit(webhookProvider.id, { secret: "next-token" });
    });
    expect(result.current.isDirty).toBe(true);

    mocks.updateNotificationProvider.mockResolvedValueOnce({
      ...webhookProvider,
      config: {
        url: GOCLAW_CLARIFICATION_URL,
        headers: { "X-Tenant": "beta" },
        secret: "next-token",
      },
    });
    await act(() => result.current.saveRequest.run());

    expect(mocks.updateNotificationProvider).toHaveBeenCalledWith(webhookProvider.id, {
      config: {
        url: GOCLAW_CLARIFICATION_URL,
        headers: { "X-Tenant": "beta" },
        secret: "next-token",
      },
    });

    act(() => result.current.actions.handleWebhookEdit(webhookProvider.id, { url: "https://bad" }));
    act(() => result.current.actions.cancelWebhookForm());
    expect(result.current.state.providers[0]?.config.url).toBe(GOCLAW_CLARIFICATION_URL);
  });
});

describe("notification save reconciliation", () => {
  it("saves when a provider has no enabled subscriptions (null events)", async () => {
    const noEventsProvider = {
      ...savedProvider,
      id: "provider-no-events",
      events: null as unknown as string[],
    };
    notificationProviders = [noEventsProvider];
    mocks.updateNotificationProvider.mockResolvedValueOnce({
      ...noEventsProvider,
      name: "Renamed provider",
      events: [],
    });
    const { result } = renderHook(useHarness);

    act(() => {
      result.current.actions.handleAppriseNameEdit(noEventsProvider.id, "Renamed provider");
    });
    await act(() => result.current.saveRequest.run());

    expect(mocks.updateNotificationProvider).toHaveBeenCalledWith(noEventsProvider.id, {
      name: "Renamed provider",
    });
  });
});
