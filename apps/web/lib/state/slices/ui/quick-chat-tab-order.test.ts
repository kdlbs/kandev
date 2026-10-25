import { describe, expect, it } from "vitest";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import {
  orderQuickChatTabs,
  resolveQuickChatTabOrder,
  type QuickChatTabDescriptor,
} from "./quick-chat-tab-order";

const OLD_CHAT_REFERENCE = "conversation:old";
const TERMINAL_REFERENCE = "terminal:one";
const TERMINAL_ONE_REFERENCE = "terminal:terminal-1";

describe("resolveQuickChatTabOrder", () => {
  it("keeps known saved tabs once and appends new tabs in baseline order", () => {
    const baseline: QuickChatTabDescriptor[] = [
      { reference: OLD_CHAT_REFERENCE, baselineOrder: 0 },
      { reference: TERMINAL_REFERENCE, baselineOrder: 1 },
      { reference: "conversation:new", baselineOrder: 2 },
    ];

    expect(
      resolveQuickChatTabOrder(baseline, [
        TERMINAL_REFERENCE,
        TERMINAL_REFERENCE,
        "conversation:missing",
        OLD_CHAT_REFERENCE,
      ]),
    ).toEqual([TERMINAL_REFERENCE, OLD_CHAT_REFERENCE, "conversation:new"]);
  });
});

describe("orderQuickChatTabs", () => {
  it("orders mixed tabs from saved references and keeps setup tabs trailing", () => {
    const sessions = [
      { sessionId: "old", workspaceId: "ws", kind: "chat" as const },
      { sessionId: "new", workspaceId: "ws", kind: "chat" as const },
      {
        sessionId: getQuickChatSetupSessionId("ws", "chat"),
        workspaceId: "ws",
        kind: "chat" as const,
      },
    ];
    const terminalTabs = [
      {
        tabId: "terminal-1",
        workspaceId: "ws",
        sessionId: null,
        sequence: 1,
        status: "running" as const,
      },
    ];

    const result = orderQuickChatTabs(sessions, terminalTabs, [
      TERMINAL_ONE_REFERENCE,
      OLD_CHAT_REFERENCE,
      "conversation:unknown",
    ]);

    expect(result.order).toEqual([TERMINAL_ONE_REFERENCE, OLD_CHAT_REFERENCE, "conversation:new"]);
    expect(result.sessions.map((session) => session.sessionId)).toEqual([
      "old",
      "new",
      getQuickChatSetupSessionId("ws", "chat"),
    ]);
    expect(result.terminalTabs.map((tab) => tab.tabId)).toEqual(["terminal-1"]);
  });
});
