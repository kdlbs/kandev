import { beforeEach, describe, expect, it } from "vitest";
import {
  ANONYMOUS_QUICK_CHAT_SELECTION_IDENTITY,
  loadQuickChatSelection,
  persistQuickChatSelection,
} from "./selection-storage";

describe("Quick Chat selection storage", () => {
  beforeEach(() => window.localStorage.clear());

  it("keeps remembered selections separate for each identity", () => {
    persistQuickChatSelection("user-a", { "workspace-a": { chat: "chat-a" } }, ["workspace-a"]);

    expect(loadQuickChatSelection("user-a").selections).toEqual({
      "workspace-a": { chat: "chat-a" },
    });
    expect(loadQuickChatSelection("user-b").selections).toEqual({});
  });

  it("retains valid entries and ignores malformed stored values", () => {
    window.localStorage.setItem(
      "kandev.quick-chat.selection.v1.user-a",
      JSON.stringify({
        version: 1,
        entries: [
          { workspaceId: "workspace-a", chat: "chat-a", lastSelectedAt: 2 },
          { workspaceId: "", chat: "invalid", lastSelectedAt: 1 },
          { workspaceId: "workspace-b", config: 42, lastSelectedAt: 0 },
        ],
      }),
    );

    expect(loadQuickChatSelection("user-a").selections).toEqual({
      "workspace-a": { chat: "chat-a" },
    });
  });

  it("uses an anonymous identity for authentication-disabled deployments", () => {
    persistQuickChatSelection(
      ANONYMOUS_QUICK_CHAT_SELECTION_IDENTITY,
      { "workspace-a": { chat: "chat-a" } },
      ["workspace-a"],
    );

    expect(loadQuickChatSelection(ANONYMOUS_QUICK_CHAT_SELECTION_IDENTITY).selections).toEqual({
      "workspace-a": { chat: "chat-a" },
    });
  });

  it("bounds stored workspaces to the most recently selected entries", () => {
    const selections = Object.fromEntries(
      Array.from({ length: 205 }, (_, index) => [`workspace-${index}`, { chat: `chat-${index}` }]),
    );
    const order = Object.keys(selections).reverse();

    persistQuickChatSelection("user-a", selections, order);

    const loaded = loadQuickChatSelection("user-a");
    expect(Object.keys(loaded.selections)).toHaveLength(200);
    expect(loaded.order[0]).toBe("workspace-204");
    expect(loaded.selections["workspace-0"]).toBeUndefined();
  });
});
