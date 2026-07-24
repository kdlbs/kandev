import { describe, it, expect } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { findUnreadDividerItemId } from "./session-unread-divider";

function makeMessage(id: string, content = ""): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content,
    type: "message",
    created_at: "",
  };
}

const messageItem = (message: Message): RenderItem => ({ type: "message", message });
const turnGroup = (id: string, messages: Message[]): RenderItem => ({
  type: "turn_group",
  id,
  turnId: messages[0]?.turn_id ?? null,
  messages,
});
const prepareProgress = (id: string): RenderItem => ({
  type: "prepare_progress",
  id,
  sessionId: "s1",
});

describe("findUnreadDividerItemId", () => {
  it("returns null when there is no read cursor yet", () => {
    const items = [messageItem(makeMessage("m1"))];
    expect(findUnreadDividerItemId(items, null)).toBeNull();
    expect(findUnreadDividerItemId(items, undefined)).toBeNull();
    expect(findUnreadDividerItemId(items, "")).toBeNull();
  });

  it("places the divider before the next standalone message after the cursor", () => {
    const items = [
      messageItem(makeMessage("m1")),
      messageItem(makeMessage("m2")),
      messageItem(makeMessage("m3")),
    ];
    expect(findUnreadDividerItemId(items, "m1")).toBe("m2");
  });

  it("returns null when the cursor already points at the newest message", () => {
    const items = [messageItem(makeMessage("m1")), messageItem(makeMessage("m2"))];
    expect(findUnreadDividerItemId(items, "m2")).toBeNull();
  });

  it("treats a turn_group as a single unit — a cursor inside the group places the divider after the whole group, not mid-group", () => {
    const a1 = makeMessage("a1");
    const a2 = makeMessage("a2");
    const a3 = makeMessage("a3");
    const items = [turnGroup("turn-group-a1", [a1, a2, a3]), messageItem(makeMessage("m-after"))];
    // Cursor sits on the middle message of the group — the divider must not
    // split the group, so it lands before the next item entirely.
    expect(findUnreadDividerItemId(items, "a2")).toBe("m-after");
  });

  it("returns null when the cursor is the last message inside the newest turn_group", () => {
    const a1 = makeMessage("a1");
    const a2 = makeMessage("a2");
    const items = [turnGroup("turn-group-a1", [a1, a2])];
    expect(findUnreadDividerItemId(items, "a2")).toBeNull();
  });

  it("skips non-message items (prepare_progress) when locating the next unread message", () => {
    const items = [
      messageItem(makeMessage("m1")),
      prepareProgress("prepare-1"),
      messageItem(makeMessage("m2")),
    ];
    expect(findUnreadDividerItemId(items, "m1")).toBe("m2");
  });

  it("treats a cursor missing from the loaded window as the whole window being unread, placing the divider before its first message", () => {
    // Simulates returning to an old task: the persisted cursor points at a
    // message far older than what pagination has loaded so far.
    const items = [messageItem(makeMessage("m10")), messageItem(makeMessage("m11"))];
    expect(findUnreadDividerItemId(items, "m1-not-loaded")).toBe("m10");
  });
});
