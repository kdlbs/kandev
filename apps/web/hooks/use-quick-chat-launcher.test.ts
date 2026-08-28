import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const requestQuickChatClose = vi.hoisted(() => vi.fn());

vi.mock("@/components/quick-chat/quick-chat-focus", () => ({
  captureQuickChatLauncherFocus: vi.fn(),
  requestQuickChatClose,
}));

const openQuickChat = vi.fn();
const closeQuickChat = vi.fn();
const WORKSPACE_ID = "workspace-1";
const ACTIVE_CONFIG_ID = "config-active";
let activeSessionId: string | null = null;
let isOpen = false;
let sessions: Array<{
  sessionId: string;
  workspaceId: string;
  kind: "chat" | "config";
}> = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ openQuickChat, closeQuickChat, quickChat: { isOpen, sessions, activeSessionId } }),
}));

import { useQuickChatLauncher } from "./use-quick-chat-launcher";

beforeEach(() => {
  sessions = [];
  activeSessionId = null;
  isOpen = false;
  openQuickChat.mockReset();
  closeQuickChat.mockReset();
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
    const { result } = renderHook(() => useQuickChatLauncher(WORKSPACE_ID, "config"));

    act(() => result.current());

    expect(openQuickChat).toHaveBeenCalledWith(ACTIVE_CONFIG_ID, WORKSPACE_ID, undefined, "config");
  });
});
