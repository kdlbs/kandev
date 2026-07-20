import { describe, expect, it } from "vitest";
import { getQuickChatSetupSessionId, isQuickChatSetupSessionId } from "./quick-chat-session";

describe("quick chat setup session IDs", () => {
  it.each(["chat", "config"] as const)("recognizes a generated %s setup ID", (kind) => {
    expect(isQuickChatSetupSessionId(getQuickChatSetupSessionId("workspace-1", kind))).toBe(true);
  });

  it.each([
    "quick-chat-setup:",
    "quick-chat-setup:persisted-session",
    "quick-chat-setup:workspace-1:other",
    "quick-chat-setup::chat",
  ])("does not classify persisted session ID %s as setup", (sessionId) => {
    expect(isQuickChatSetupSessionId(sessionId)).toBe(false);
  });
});
