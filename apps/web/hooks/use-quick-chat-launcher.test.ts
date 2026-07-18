import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const openQuickChat = vi.fn();
const WORKSPACE_ID = "workspace-1";
const ACTIVE_CONFIG_ID = "config-active";
let activeSessionId: string | null = null;
let sessions: Array<{
  sessionId: string;
  workspaceId: string;
  kind: "chat" | "config";
}> = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ openQuickChat, quickChat: { sessions, activeSessionId } }),
}));

import { useQuickChatLauncher } from "./use-quick-chat-launcher";

beforeEach(() => {
  sessions = [];
  activeSessionId = null;
  openQuickChat.mockReset();
});

describe("useQuickChatLauncher typed sessions", () => {
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
