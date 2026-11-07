import { describe, expect, it } from "vitest";
import { isQuickChatStartupPending } from "./quick-chat-startup-state";

describe("isQuickChatStartupPending", () => {
  it.each(["CREATED", "STARTING"] as const)("guards accepted %s startup", (sessionState) => {
    expect(isQuickChatStartupPending({ sessionState })).toBe(true);
  });

  it("uses the stored acceptance state until a session row arrives", () => {
    expect(isQuickChatStartupPending({ startupPending: true })).toBe(true);
  });

  it("releases the startup guard after lifecycle state advances", () => {
    expect(
      isQuickChatStartupPending({
        startupPending: true,
        sessionState: "WAITING_FOR_INPUT",
      }),
    ).toBe(false);
  });

  it("does not gate sessions without startup evidence", () => {
    expect(isQuickChatStartupPending({})).toBe(false);
  });

  it("guards a new Quick Chat before its creation identity reaches the lifecycle event", () => {
    expect(isQuickChatStartupPending({ sessionState: "CREATED", startupPending: true })).toBe(true);
  });
});
