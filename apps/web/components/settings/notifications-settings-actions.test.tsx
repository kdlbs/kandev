import { act, renderHook } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
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
const PAGER_URL = "json://pager";

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

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: [savedProvider],
    events: ["task.completed"],
    appriseAvailable: true,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ setNotificationProviders: mocks.setNotificationProviders }),
}));

function useHarness() {
  const state = useNotificationsState();
  const saveRequest = useSaveRequest(state);
  const actions = useNotificationsActions(state, vi.fn());
  return { state, saveRequest, actions, isDirty: useIsDirty(state) };
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: makeQueryClient() }, children);
}

function renderHarness() {
  return renderHook(useHarness, { wrapper });
}

const createdProvider: NotificationProvider = {
  ...savedProvider,
  id: "provider-2",
  name: "Pager",
  config: { urls: [PAGER_URL] },
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.createNotificationProvider.mockResolvedValue(createdProvider);
});

describe("notification provider drafts", () => {
  it("stages a new Apprise provider until the shared save runs", async () => {
    const { result } = renderHarness();

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
      }),
    );
    expect(result.current.state.providers).toContainEqual(createdProvider);
    expect(result.current.state.showAppriseForm).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });

  it("keeps a failed create draft open and dirty", async () => {
    mocks.createNotificationProvider.mockRejectedValueOnce(new Error("create unavailable"));
    const { result } = renderHarness();
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
    const { result } = renderHarness();
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
    const { result } = renderHarness();
    act(() => result.current.actions.handleAppriseNameEdit(savedProvider.id, "Changed"));
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.actions.discard());

    expect(result.current.state.providers).toEqual([savedProvider]);
    expect(result.current.isDirty).toBe(false);
  });
});
