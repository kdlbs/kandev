import { describe, expect, it, vi } from "vitest";
import { createDemoConversationRuntime } from "./conversation-runtime";

const SUBSCRIBE = "session.conversation.subscribe";
const MESSAGE_ADDED = "session.message.added";
const payload = { scope_id: "scope", session_id: "session" };

describe("browser demo conversation protocol", () => {
  it("acknowledges the source stream with the current epoch and decimal revision", () => {
    const runtime = createDemoConversationRuntime(vi.fn());
    expect(runtime.route("socket", SUBSCRIBE, payload)).toMatchObject({
      success: true,
      protocol_version: 2,
      ...payload,
      epoch: expect.any(String),
      revision: "0",
    });
  });

  it("publishes ordered message changes only to subscribed sessions", () => {
    const send = vi.fn();
    const runtime = createDemoConversationRuntime(send);
    runtime.route("socket", SUBSCRIBE, payload);
    runtime.route("other", SUBSCRIBE, { ...payload, session_id: "other-session" });
    const message = { session_id: "session", message_id: "message", content: "Working" };
    runtime.publish(MESSAGE_ADDED, message);
    runtime.publish("session.message.updated", { ...message, content: "Ready for review" });
    expect(send).toHaveBeenCalledTimes(2);
    expect(send).toHaveBeenLastCalledWith(
      "socket",
      expect.objectContaining({
        action: "session.conversation.changed",
        payload: expect.objectContaining({
          base_revision: "1",
          revision: "2",
          operations: [
            {
              kind: "upsert",
              entity: "message",
              id: "message",
              message: { ...message, content: "Ready for review" },
            },
          ],
        }),
      }),
    );
    expect(runtime.route("socket", SUBSCRIBE, payload)).toMatchObject({ revision: "2" });
  });

  it("removes subscriptions on unsubscribe and socket close", () => {
    const send = vi.fn();
    const runtime = createDemoConversationRuntime(send);
    runtime.route("socket", SUBSCRIBE, payload);
    runtime.route("socket", "session.conversation.unsubscribe", payload);
    runtime.publish(MESSAGE_ADDED, { session_id: "session", message_id: "message" });
    runtime.route("socket", SUBSCRIBE, payload);
    runtime.close("socket");
    runtime.publish(MESSAGE_ADDED, { session_id: "session", message_id: "message" });
    expect(send).not.toHaveBeenCalled();
  });
});
