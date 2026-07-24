import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageActions } from "./message-actions";

const MESSAGE_TIMESTAMP = "2026-07-20T10:15:00Z";

function assistantMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "hello",
    type: "message",
    created_at: MESSAGE_TIMESTAMP,
    ...overrides,
  };
}

afterEach(cleanup);

describe("MessageActions timestamp tooltip", () => {
  it("renders the relative timestamp as a <time> element with the full absolute time as its title", () => {
    const { container } = render(
      <StateProvider>
        <MessageActions message={assistantMessage()} />
      </StateProvider>,
    );

    const timeEl = container.querySelector("time");
    expect(timeEl).not.toBeNull();
    expect(timeEl?.getAttribute("dateTime")).toBe(MESSAGE_TIMESTAMP);
    expect(timeEl?.getAttribute("title")).toBe(new Date(MESSAGE_TIMESTAMP).toLocaleString());
  });

  it("omits the timestamp element entirely when showTimestamp is false", () => {
    const { container } = render(
      <StateProvider>
        <MessageActions message={assistantMessage()} showTimestamp={false} />
      </StateProvider>,
    );

    expect(container.querySelector("time")).toBeNull();
  });
});
