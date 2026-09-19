import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { defaultUIState } from "@/lib/state/slices/ui/ui-slice";
import { hydrateUI } from "@/lib/state/hydration/hydrator";
import type { AppState } from "@/lib/state/store";

const requestQuickChatClose = vi.hoisted(() => vi.fn());

vi.mock("@/components/quick-chat/quick-chat-focus", () => ({
  captureQuickChatLauncherFocus: vi.fn(),
  requestQuickChatClose,
}));

const openQuickChat = vi.fn();
const closeQuickChat = vi.fn();
const requestQuickChatOpen = vi.fn();
const WORKSPACE_ID = "workspace-1";
const ACTIVE_CONFIG_ID = "config-active";
const REMEMBERED_CHAT_ID = "chat-remembered";
let activeSessionId: string | null = null;
let isOpen = false;
let rememberedSelectionByWorkspace: Record<string, { chat?: string; config?: string }> = {};
let selectionReadyByWorkspace: Record<string, boolean> = { [WORKSPACE_ID]: true };
let selectionRevisionByWorkspace: Record<string, number> = {};
let quickChatTabOrderByWorkspace: Record<string, string[]> = {};
let sessions: Array<{
  sessionId: string;
  workspaceId: string;
  kind: "chat" | "config";
}> = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      openQuickChat,
      closeQuickChat,
      requestQuickChatOpen,
      quickChat: {
        isOpen,
        sessions,
        activeSessionId,
        rememberedSelectionByWorkspace,
        selectionReadyByWorkspace,
        selectionRevisionByWorkspace,
        tabOrderByWorkspace: {},
      },
      userSettings: { quickChatTabOrderByWorkspace },
    }),
}));

import { useQuickChatLauncher } from "./use-quick-chat-launcher";

beforeEach(() => {
  sessions = [];
  activeSessionId = null;
  isOpen = false;
  rememberedSelectionByWorkspace = {};
  selectionReadyByWorkspace = { [WORKSPACE_ID]: true };
  selectionRevisionByWorkspace = {};
  quickChatTabOrderByWorkspace = {};
  openQuickChat.mockReset();
  closeQuickChat.mockReset();
  requestQuickChatOpen.mockReset();
  requestQuickChatClose.mockReset();
  requestQuickChatClose.mockReturnValue(false);
});

describe("useQuickChatLauncher typed sessions", () => {
  it("@covers AC-UI-QUICK-TERMINAL-001.10 closes an open dialog when toggle behavior is enabled", () => {
    isOpen = true;
    requestQuickChatClose.mockReturnValue(true);
    const { result } = renderHook(() =>
      useQuickChatLauncher(WORKSPACE_ID, "chat", { toggleWhenOpen: true }),
    );

    act(() => result.current());

    expect(requestQuickChatClose).toHaveBeenCalledTimes(1);
    expect(closeQuickChat).not.toHaveBeenCalled();
    expect(openQuickChat).not.toHaveBeenCalled();
  });

  it("keeps ordinary launchers open-only when the dialog is already open", () => {
    isOpen = true;
    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(closeQuickChat).not.toHaveBeenCalled();
    expect(openQuickChat).toHaveBeenCalledWith("", WORKSPACE_ID, undefined, "chat");
  });

  it("opens an ordinary session from the generic quick chat launcher", () => {
    sessions = [
      { sessionId: "chat-1", workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: ACTIVE_CONFIG_ID, workspaceId: WORKSPACE_ID, kind: "config" },
    ];
    activeSessionId = ACTIVE_CONFIG_ID;
    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith("chat-1", WORKSPACE_ID, undefined, "chat");
  });

  it("opens the matching ordinary session when config and chat tabs coexist", () => {
    sessions = [
      { sessionId: "config-1", workspaceId: WORKSPACE_ID, kind: "config" },
      { sessionId: "chat-1", workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    const { result } = renderHook(() =>
      (useQuickChatLauncher as (...args: unknown[]) => () => void)(WORKSPACE_ID, "chat"),
    );

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith("chat-1", WORKSPACE_ID, undefined, "chat");
  });

  it("opens a typed config setup when no config session exists", () => {
    sessions = [{ sessionId: "chat-1", workspaceId: WORKSPACE_ID, kind: "chat" }];
    const { result } = renderHook(() =>
      (useQuickChatLauncher as (...args: unknown[]) => () => void)(WORKSPACE_ID, "config"),
    );

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith("", WORKSPACE_ID, undefined, "config");
  });

  it("prefers the active matching session over the first restored session", () => {
    sessions = [
      { sessionId: "config-newest", workspaceId: WORKSPACE_ID, kind: "config" },
      { sessionId: ACTIVE_CONFIG_ID, workspaceId: WORKSPACE_ID, kind: "config" },
    ];
    activeSessionId = ACTIVE_CONFIG_ID;
    selectionRevisionByWorkspace = { [WORKSPACE_ID]: 1 };
    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID, "config"));

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith(ACTIVE_CONFIG_ID, WORKSPACE_ID, undefined, "config");
  });

  it("restores the remembered conversation after visiting another workspace", () => {
    sessions = [
      { sessionId: "chat-a-first", workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: "chat-a-remembered", workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: "chat-b-active", workspaceId: "workspace-2", kind: "chat" },
    ];
    activeSessionId = "chat-b-active";
    rememberedSelectionByWorkspace = { [WORKSPACE_ID]: { chat: "chat-a-remembered" } };

    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith(
      "chat-a-remembered",
      WORKSPACE_ID,
      undefined,
      "chat",
    );
  });
});

describe("useQuickChatLauncher delayed selection", () => {
  it("launches the tab that hydration restored from browser selection", () => {
    const hydrated = produce(
      structuredClone(defaultUIState) as unknown as AppState,
      (draft: Draft<AppState>) => {
        draft.quickChat.rememberedSelectionByWorkspace = {
          [WORKSPACE_ID]: { chat: REMEMBERED_CHAT_ID },
        };
        draft.quickChat.rememberedSelectionOrder = [WORKSPACE_ID];
        hydrateUI(draft, {
          quickChat: {
            isOpen: false,
            activeSessionId: null,
            sessions: [
              { sessionId: "chat-first", workspaceId: WORKSPACE_ID, kind: "chat" },
              { sessionId: REMEMBERED_CHAT_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
            ],
          },
        });
      },
    );
    sessions = hydrated.quickChat.sessions;
    activeSessionId = hydrated.quickChat.activeSessionId;
    rememberedSelectionByWorkspace = hydrated.quickChat.rememberedSelectionByWorkspace;
    selectionReadyByWorkspace = hydrated.quickChat.selectionReadyByWorkspace;
    selectionRevisionByWorkspace = hydrated.quickChat.selectionRevisionByWorkspace;

    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(activeSessionId).toBe(REMEMBERED_CHAT_ID);
    expect(openQuickChat).toHaveBeenCalledWith(REMEMBERED_CHAT_ID, WORKSPACE_ID, undefined, "chat");
  });

  it("waits for the authoritative workspace list before choosing a fallback", () => {
    sessions = [{ sessionId: "chat-other", workspaceId: "workspace-2", kind: "chat" }];
    selectionReadyByWorkspace = {};

    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(requestQuickChatOpen).toHaveBeenCalledWith(WORKSPACE_ID, "chat");
    expect(openQuickChat).not.toHaveBeenCalled();
  });

  it("passes the persisted tab order through a delayed open", () => {
    sessions = [{ sessionId: "chat-other", workspaceId: "workspace-2", kind: "chat" }];
    selectionReadyByWorkspace = {};
    quickChatTabOrderByWorkspace = {
      [WORKSPACE_ID]: ["conversation:chat-second", "conversation:chat-first"],
    };

    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID));

    act(() => result.current());

    expect(requestQuickChatOpen).toHaveBeenCalledWith(WORKSPACE_ID, "chat", [
      "conversation:chat-second",
      "conversation:chat-first",
    ]);
  });
});
