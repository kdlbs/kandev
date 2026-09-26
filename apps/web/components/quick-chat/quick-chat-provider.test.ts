import { describe, expect, it } from "vitest";
import { getWorkspaceId } from "./quick-chat-provider";

const sessions = [
  { sessionId: "session-a", workspaceId: "ws-a" },
  { sessionId: "session-b", workspaceId: "ws-b" },
];
const terminalTabs = [{ tabId: "terminal-a", workspaceId: "ws-a" }];

describe("getWorkspaceId", () => {
  it("uses the active tab workspace instead of the first persisted tab", () => {
    expect(
      getWorkspaceId({
        sessions,
        terminalTabs,
        isOpen: true,
        activeKind: "conversation",
        activeSessionId: "session-b",
        activeTerminalTabId: null,
        activeWorkspace: "ws-a",
      }),
    ).toBe("ws-b");
  });

  it("uses the active workspace for a new-chat placeholder", () => {
    expect(
      getWorkspaceId({
        sessions,
        terminalTabs,
        isOpen: true,
        activeKind: "conversation",
        activeSessionId: "",
        activeTerminalTabId: null,
        activeWorkspace: "ws-b",
      }),
    ).toBe("ws-b");
  });

  it("uses the active terminal workspace when a terminal is selected", () => {
    expect(
      getWorkspaceId({
        sessions,
        terminalTabs,
        isOpen: true,
        activeKind: "terminal",
        activeSessionId: "session-b",
        activeTerminalTabId: "terminal-a",
        activeWorkspace: "ws-b",
      }),
    ).toBe("ws-a");
  });

  it("does not mount a closed dialog from stale sessions", () => {
    expect(
      getWorkspaceId({
        sessions,
        terminalTabs,
        isOpen: false,
        activeKind: "conversation",
        activeSessionId: "session-a",
        activeTerminalTabId: null,
        activeWorkspace: "ws-a",
      }),
    ).toBeNull();
  });
});
