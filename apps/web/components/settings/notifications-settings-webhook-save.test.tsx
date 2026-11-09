import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NotificationProvider } from "@/lib/types/http";
import {
  useIsDirty,
  useNotificationsActions,
  useNotificationsState,
  useSaveRequest,
} from "./notifications-settings-actions";

const mocks = vi.hoisted(() => ({
  createNotificationProvider: vi.fn(),
  deleteNotificationProvider: vi.fn(),
  testNotificationProvider: vi.fn(),
  updateNotificationProvider: vi.fn(),
  setNotificationProviders: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  createNotificationProvider: mocks.createNotificationProvider,
  deleteNotificationProvider: mocks.deleteNotificationProvider,
  testNotificationProvider: mocks.testNotificationProvider,
  updateNotificationProvider: mocks.updateNotificationProvider,
}));

const SAVED_EVENT = "task.completed";
const NOTIFICATION_EVENTS = [SAVED_EVENT];
const WEBHOOK_TEST_URL = "https://hooks.example.com/kandev";
const WEBHOOK_TEST_NAME = "Inbound hooks";
const WEBHOOK_HEADER_DRAFT = "X-Tenant: beta";
const WEBHOOK_SUBMITTED_HEADER_DRAFT = "X-Tenant: submitted";
const WEBHOOK_NEWER_HEADER_DRAFT = "X-Tenant: latest";
const WEBHOOK_HEADERS_INVALID_MESSAGE =
  "Webhook headers must use valid HTTP header names and must not repeat.";

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

const anotherWebhookProvider: NotificationProvider = {
  ...webhookProvider,
  id: "provider-webhook-2",
  name: "Regional hooks",
  config: { url: "https://region.example/hooks", headers: { "X-Region": "eu" } },
};

let notificationProviders = [savedProvider];

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: notificationProviders,
    events: NOTIFICATION_EVENTS,
    appriseAvailable: true,
    loaded: true,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ setNotificationProviders: mocks.setNotificationProviders }),
}));

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: () => false,
    permission: { get: vi.fn(), request: vi.fn() },
  },
}));

function useHarness() {
  const state = useNotificationsState();
  const saveRequest = useSaveRequest(state);
  const actions = useNotificationsActions(state, vi.fn());
  return { state, saveRequest, actions, isDirty: useIsDirty(state) };
}

beforeEach(() => {
  vi.clearAllMocks();
  notificationProviders = [savedProvider];
  mocks.createNotificationProvider.mockResolvedValue({
    ...webhookProvider,
    id: "provider-webhook-new",
    name: WEBHOOK_TEST_NAME,
  });
});

describe("webhook provider save safeguards", () => {
  it("refuses to save webhook headers that the backend would reject", async () => {
    const { result } = renderHook(useHarness);
    act(() => {
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
      result.current.state.setWebhookHeaders("X-Tenant: acme\nx-tenant: other");
    });

    await act(async () => {
      await expect(result.current.saveRequest.run()).rejects.toThrow(
        WEBHOOK_HEADERS_INVALID_MESSAGE,
      );
    });

    expect(mocks.createNotificationProvider).not.toHaveBeenCalled();
    expect(result.current.state.showWebhookForm).toBe(true);
  });

  it("keeps only the most recently opened create form", async () => {
    const { result } = renderHook(useHarness);

    act(() => {
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseName("Pager");
      result.current.state.setAppriseUrls("json://pager");
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
    });
    expect(result.current.state.showAppriseForm).toBe(false);
    expect(result.current.state.showWebhookForm).toBe(true);

    act(() => {
      result.current.actions.openAppriseForm("create");
      result.current.state.setAppriseUrls("json://pager");
    });
    expect(result.current.state.showWebhookForm).toBe(false);
    expect(result.current.state.showAppriseForm).toBe(true);

    act(() => {
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
    });
    await act(() => result.current.saveRequest.run());

    expect(mocks.createNotificationProvider).toHaveBeenCalledOnce();
    expect(mocks.createNotificationProvider).toHaveBeenCalledWith(
      expect.objectContaining({ type: "webhook" }),
    );
  });
});

describe("webhook provider edit safeguards", () => {
  it("keeps an invalid edit open when Done is selected", () => {
    notificationProviders = [webhookProvider];
    const { result } = renderHook(useHarness);
    const invalidHeaders = "Authorization Bearer malformed";

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, { headers: invalidHeaders });
      result.current.state.setWebhookHeaders(invalidHeaders);
    });
    act(() => result.current.actions.closeWebhookForm());

    expect(result.current.state.showWebhookForm).toBe(true);
    expect(result.current.state.activeWebhookId).toBe(webhookProvider.id);
    expect(result.current.state.webhookHeaders).toBe(invalidHeaders);
  });

  it("restores each provider's raw header draft when edit forms switch", () => {
    notificationProviders = [webhookProvider, anotherWebhookProvider];
    const { result } = renderHook(useHarness);

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, {
        headers: WEBHOOK_HEADER_DRAFT,
      });
      result.current.state.setWebhookHeaders(WEBHOOK_HEADER_DRAFT);
    });
    act(() => result.current.actions.openWebhookForm("edit", anotherWebhookProvider));

    expect(result.current.state.activeWebhookId).toBe(anotherWebhookProvider.id);
    expect(result.current.state.webhookHeaders).toBe("X-Region: eu");

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    expect(result.current.state.webhookHeaders).toBe(WEBHOOK_HEADER_DRAFT);
  });

  it("blocks Save when a switched-away edit has invalid headers", async () => {
    notificationProviders = [webhookProvider, anotherWebhookProvider];
    const { result } = renderHook(useHarness);
    const invalidHeaders = "Authorization Bearer malformed";

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, { headers: invalidHeaders });
      result.current.state.setWebhookHeaders(invalidHeaders);
    });
    act(() => result.current.actions.openWebhookForm("edit", anotherWebhookProvider));

    await act(async () => {
      await expect(result.current.saveRequest.run()).rejects.toThrow(
        WEBHOOK_HEADERS_INVALID_MESSAGE,
      );
    });
    expect(mocks.updateNotificationProvider).not.toHaveBeenCalled();
  });
});

describe("webhook provider edit save reconciliation", () => {
  it("keeps a newer raw header draft when an edit save finishes", async () => {
    let resolveUpdate: (provider: NotificationProvider) => void = () => undefined;
    const updatePromise = new Promise<NotificationProvider>((resolve) => {
      resolveUpdate = resolve;
    });
    mocks.updateNotificationProvider.mockReturnValueOnce(updatePromise);
    notificationProviders = [webhookProvider];
    const { result } = renderHook(useHarness);

    act(() => result.current.actions.openWebhookForm("edit", webhookProvider));
    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, {
        headers: WEBHOOK_SUBMITTED_HEADER_DRAFT,
      });
      result.current.state.setWebhookHeaders(WEBHOOK_SUBMITTED_HEADER_DRAFT);
    });

    let savePromise: Promise<unknown>;
    act(() => {
      savePromise = result.current.saveRequest.run();
    });
    await waitFor(() => expect(mocks.updateNotificationProvider).toHaveBeenCalledOnce());

    act(() => {
      result.current.actions.handleWebhookEdit(webhookProvider.id, {
        headers: WEBHOOK_NEWER_HEADER_DRAFT,
      });
      result.current.state.setWebhookHeaders(WEBHOOK_NEWER_HEADER_DRAFT);
    });

    await act(async () => {
      resolveUpdate({
        ...webhookProvider,
        config: { ...webhookProvider.config, headers: { "X-Tenant": "submitted" } },
      });
      await savePromise;
    });

    expect(result.current.state.webhookHeaderDrafts[webhookProvider.id]).toBe(
      WEBHOOK_NEWER_HEADER_DRAFT,
    );
    expect(result.current.state.providers[0].config.headers).toEqual({ "X-Tenant": "latest" });
  });
});

describe("webhook provider save reconciliation", () => {
  it("does not clear a reopened webhook form with identical values", async () => {
    let resolveCreate: (provider: NotificationProvider) => void = () => undefined;
    const createPromise = new Promise<NotificationProvider>((resolve) => {
      resolveCreate = resolve;
    });
    mocks.createNotificationProvider.mockReturnValueOnce(createPromise);
    const { result } = renderHook(useHarness);

    act(() => {
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
    });

    let savePromise: Promise<unknown>;
    act(() => {
      savePromise = result.current.saveRequest.run();
    });
    await waitFor(() => expect(mocks.createNotificationProvider).toHaveBeenCalledOnce());

    act(() => {
      result.current.actions.closeWebhookForm();
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
    });

    await act(async () => {
      resolveCreate({
        ...webhookProvider,
        id: "provider-webhook-new",
        name: WEBHOOK_TEST_NAME,
      });
      await savePromise;
    });

    expect(result.current.state.showWebhookForm).toBe(true);
    expect(result.current.state.webhookName).toBe(WEBHOOK_TEST_NAME);
  });

  it("keeps newer webhook create edits made while the create request is in flight", async () => {
    let resolveCreate: (provider: NotificationProvider) => void = () => undefined;
    const createPromise = new Promise<NotificationProvider>((resolve) => {
      resolveCreate = resolve;
    });
    mocks.createNotificationProvider.mockReturnValueOnce(createPromise);
    const { result } = renderHook(useHarness);

    act(() => {
      result.current.actions.openWebhookForm("create");
      result.current.state.setWebhookName(WEBHOOK_TEST_NAME);
      result.current.state.setWebhookUrl(WEBHOOK_TEST_URL);
      result.current.state.setWebhookHeaders("X-Tenant: acme");
      result.current.state.setWebhookSecret("first-token");
    });

    let savePromise: Promise<unknown>;
    act(() => {
      savePromise = result.current.saveRequest.run();
    });
    await waitFor(() => expect(mocks.createNotificationProvider).toHaveBeenCalledOnce());

    act(() => {
      result.current.state.setWebhookHeaders(WEBHOOK_HEADER_DRAFT);
      result.current.state.setWebhookSecret("second-token");
    });

    await act(async () => {
      resolveCreate({
        ...webhookProvider,
        id: "provider-webhook-new",
        name: WEBHOOK_TEST_NAME,
      });
      await savePromise;
    });

    expect(result.current.state.showWebhookForm).toBe(true);
    expect(result.current.state.webhookHeaders).toBe(WEBHOOK_HEADER_DRAFT);
    expect(result.current.state.webhookSecret).toBe("second-token");
  });
});
