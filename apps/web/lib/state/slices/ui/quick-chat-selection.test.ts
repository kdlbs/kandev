import { describe, expect, it } from "vitest";
import type { QuickChatSession } from "./types";
import {
  clearRememberedQuickChatSelection,
  findRememberedQuickChatSession,
  rememberQuickChatSelection,
  restoreQuickChatSession,
} from "./quick-chat-selection";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const sessions: QuickChatSession[] = [
  { sessionId: "chat-first", workspaceId: WORKSPACE_A, kind: "chat" },
  { sessionId: "chat-second", workspaceId: WORKSPACE_A, kind: "chat" },
  { sessionId: "config-a", workspaceId: WORKSPACE_A, kind: "config" },
  { sessionId: "chat-other", workspaceId: WORKSPACE_B, kind: "chat" },
];

describe("Quick Chat remembered selection", () => {
  it("restores the remembered session only for its workspace and kind", () => {
    const remembered = { [WORKSPACE_A]: { chat: "chat-second", config: "config-a" } };

    expect(findRememberedQuickChatSession(sessions, remembered, WORKSPACE_A, "chat")).toEqual(
      sessions[1],
    );
    expect(findRememberedQuickChatSession(sessions, remembered, WORKSPACE_B, "chat")).toBe(
      undefined,
    );
    expect(findRememberedQuickChatSession(sessions, remembered, WORKSPACE_A, "config")).toEqual(
      sessions[2],
    );
  });

  it("falls back in displayed order when the remembered session is unavailable", () => {
    const remembered = { [WORKSPACE_A]: { chat: "deleted" } };

    expect(
      restoreQuickChatSession(sessions, remembered, WORKSPACE_A, "chat", [
        "conversation:chat-second",
        "conversation:chat-first",
      ]),
    ).toBe("chat-second");
  });

  it("updates and clears one workspace-kind preference without touching siblings", () => {
    const initial = { [WORKSPACE_A]: { chat: "chat-first", config: "config-a" } };
    const remembered = rememberQuickChatSelection(initial, WORKSPACE_B, "chat", "chat-other");
    const cleared = clearRememberedQuickChatSelection(remembered, WORKSPACE_A, "chat");

    expect(cleared).toEqual({
      [WORKSPACE_A]: { config: "config-a" },
      [WORKSPACE_B]: { chat: "chat-other" },
    });
  });
});
