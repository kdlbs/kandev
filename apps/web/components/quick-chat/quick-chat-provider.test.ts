import { describe, expect, it } from "vitest";
import { getWorkspaceId } from "./quick-chat-provider";

const sessions = [
  { sessionId: "session-a", workspaceId: "ws-a" },
  { sessionId: "session-b", workspaceId: "ws-b" },
];

describe("getWorkspaceId", () => {
  it("uses the active tab workspace instead of the first persisted tab", () => {
    expect(getWorkspaceId(sessions, true, "session-b", "ws-a")).toBe("ws-b");
  });

  it("uses the active workspace for a new-chat placeholder", () => {
    expect(getWorkspaceId(sessions, true, "", "ws-b")).toBe("ws-b");
  });

  it("does not mount a closed dialog from stale sessions", () => {
    expect(getWorkspaceId(sessions, false, "session-a", "ws-a")).toBeNull();
  });
});
