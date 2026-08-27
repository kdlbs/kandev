import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { QuickChatSession, QuickTerminalTab } from "@/lib/state/slices/ui/types";

const mockUpdateUserSettings = vi.fn();
let mockAppState: ReturnType<typeof makeAppState>;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: ReturnType<typeof makeAppState>) => unknown) =>
    selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: (...args: unknown[]) => mockUpdateUserSettings(...args),
}));

import { useQuickChatTabOrder } from "./use-quick-chat-tab-order";

const WORKSPACE_ID = "workspace-1";
const CHAT_TAB_REFERENCE = "conversation:chat-1";
const TERMINAL_TAB_REFERENCE = "terminal:terminal-1";
const sessions: QuickChatSession[] = [
  { sessionId: "chat-1", workspaceId: WORKSPACE_ID, kind: "chat" },
];
const terminalTabs: QuickTerminalTab[] = [
  {
    tabId: "terminal-1",
    workspaceId: WORKSPACE_ID,
    sessionId: null,
    sequence: 1,
    status: "running",
  },
];

function makeAppState() {
  const appState = {
    userSettings: {
      quickChatTabOrderByWorkspace: {} as Record<string, string[]>,
      revision: null as number | null,
    },
    quickChat: {
      tabOrderByWorkspace: {} as Record<string, string[]>,
      tabOrderSyncErrorByWorkspace: {} as Record<string, string | null>,
      tabOrderSyncPendingByWorkspace: {} as Record<string, boolean>,
    },
    setQuickChatTabOrder: vi.fn((workspaceId: string, order: string[]) => {
      appState.quickChat.tabOrderByWorkspace[workspaceId] = [...order];
    }),
    setQuickChatTabOrderSyncState: vi.fn(),
    clearQuickChatTabOrder: vi.fn((workspaceId: string, expectedOrder: string[]) => {
      const current = appState.quickChat.tabOrderByWorkspace[workspaceId];
      if (
        current &&
        current.length === expectedOrder.length &&
        current.every((reference, index) => reference === expectedOrder[index])
      ) {
        delete appState.quickChat.tabOrderByWorkspace[workspaceId];
      }
    }),
    setUserSettings: vi.fn((settings: typeof appState.userSettings) => {
      appState.userSettings = settings;
    }),
  };
  return appState;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockAppState = makeAppState();
  mockUpdateUserSettings.mockResolvedValue({});
});

describe("useQuickChatTabOrder", () => {
  it("serializes saves and sends the latest order after an earlier save settles", async () => {
    let releaseFirst!: () => void;
    const firstSave = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });
    mockUpdateUserSettings.mockReturnValueOnce(firstSave).mockResolvedValue(undefined);

    const { result } = renderHook(() => useQuickChatTabOrder(WORKSPACE_ID, sessions, terminalTabs));

    act(() => {
      result.current.persistOrder([TERMINAL_TAB_REFERENCE, CHAT_TAB_REFERENCE]);
    });
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.persistOrder([CHAT_TAB_REFERENCE, TERMINAL_TAB_REFERENCE]);
    });
    expect(mockUpdateUserSettings).toHaveBeenCalledTimes(1);

    releaseFirst();
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledTimes(2));

    expect(mockUpdateUserSettings).toHaveBeenNthCalledWith(2, {
      quick_chat_tab_order_by_workspace: {
        [WORKSPACE_ID]: [CHAT_TAB_REFERENCE, TERMINAL_TAB_REFERENCE],
      },
    });
  });

  it("keeps the optimistic order and reports the latest save failure", async () => {
    mockUpdateUserSettings.mockRejectedValue(new Error("settings unavailable"));

    const { result } = renderHook(() => useQuickChatTabOrder(WORKSPACE_ID, sessions, terminalTabs));
    const order = [TERMINAL_TAB_REFERENCE, CHAT_TAB_REFERENCE];

    act(() => result.current.persistOrder(order));
    await waitFor(() =>
      expect(mockAppState.setQuickChatTabOrderSyncState).toHaveBeenLastCalledWith(WORKSPACE_ID, {
        pending: false,
        error: "settings unavailable",
      }),
    );

    expect(mockAppState.setQuickChatTabOrder).toHaveBeenCalledWith(WORKSPACE_ID, order);
    expect(mockUpdateUserSettings).toHaveBeenCalledWith({
      quick_chat_tab_order_by_workspace: { [WORKSPACE_ID]: order },
    });
  });

  it("reveals a newer persisted order after the latest local save succeeds", async () => {
    const localOrder = [TERMINAL_TAB_REFERENCE, CHAT_TAB_REFERENCE];
    const newerServerOrder = [CHAT_TAB_REFERENCE, TERMINAL_TAB_REFERENCE];
    mockUpdateUserSettings.mockResolvedValue({
      settings: {
        user_id: "user-1",
        workspace_id: "",
        repository_ids: [],
        quick_chat_tab_order_by_workspace: { [WORKSPACE_ID]: localOrder },
        revision: 1,
        updated_at: "2026-08-27T00:00:00Z",
      },
    });

    const { result, rerender } = renderHook(() =>
      useQuickChatTabOrder(WORKSPACE_ID, sessions, terminalTabs),
    );

    act(() => result.current.persistOrder(localOrder));
    await waitFor(() => expect(mockAppState.setUserSettings).toHaveBeenCalledOnce());

    // A newer user.settings.updated event arrives after the local response.
    mockAppState.userSettings = {
      ...mockAppState.userSettings,
      quickChatTabOrderByWorkspace: { [WORKSPACE_ID]: newerServerOrder },
      revision: 2,
    };
    rerender();

    expect(result.current.order).toEqual(newerServerOrder);
  });

  it("persists removal of a closed tab reference", async () => {
    const savedOrder = [CHAT_TAB_REFERENCE, TERMINAL_TAB_REFERENCE];
    mockAppState.userSettings.quickChatTabOrderByWorkspace = {
      [WORKSPACE_ID]: savedOrder,
    };

    const { result } = renderHook(() => useQuickChatTabOrder(WORKSPACE_ID, sessions, terminalTabs));

    act(() => result.current.removeTabReference(CHAT_TAB_REFERENCE));
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledOnce());

    expect(mockUpdateUserSettings).toHaveBeenCalledWith({
      quick_chat_tab_order_by_workspace: {
        [WORKSPACE_ID]: [TERMINAL_TAB_REFERENCE],
      },
    });
  });
});

describe("useQuickChatTabOrder — workspace isolation", () => {
  it("does not resend another workspace's failed optimistic order", async () => {
    const otherWorkspaceId = "workspace-2";
    const persistedOtherOrder = ["conversation:other-server"];
    const failedOtherOrder = ["terminal:other-failed"];
    mockAppState.userSettings.quickChatTabOrderByWorkspace = {
      [otherWorkspaceId]: persistedOtherOrder,
    };
    mockAppState.quickChat.tabOrderByWorkspace = {
      [otherWorkspaceId]: failedOtherOrder,
    };

    const localOrder = [TERMINAL_TAB_REFERENCE, CHAT_TAB_REFERENCE];
    const { result } = renderHook(() => useQuickChatTabOrder(WORKSPACE_ID, sessions, terminalTabs));

    act(() => result.current.persistOrder(localOrder));
    await waitFor(() => expect(mockUpdateUserSettings).toHaveBeenCalledOnce());

    expect(mockUpdateUserSettings).toHaveBeenCalledWith({
      quick_chat_tab_order_by_workspace: {
        [WORKSPACE_ID]: localOrder,
        [otherWorkspaceId]: persistedOtherOrder,
      },
    });
  });
});
