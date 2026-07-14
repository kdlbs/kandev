import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const openQuickChat = vi.fn();
const WORKSPACE_ID = "workspace-1";
let sessions: Array<{
  sessionId: string;
  workspaceId: string;
  kind: "chat" | "config";
}> = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ openQuickChat, quickChat: { sessions } }),
}));

import { useQuickChatLauncher } from "./use-quick-chat-launcher";

beforeEach(() => {
  sessions = [];
  openQuickChat.mockReset();
});

describe("useQuickChatLauncher typed sessions", () => {
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
});
