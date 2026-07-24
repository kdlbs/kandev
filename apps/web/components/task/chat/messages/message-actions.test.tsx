import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageActions } from "./message-actions";

const TOUCH_DRAWER = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => TOUCH_DRAWER.enabled,
}));

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

afterEach(() => {
  TOUCH_DRAWER.enabled = false;
  cleanup();
});

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

describe("MessageActions timestamp tooltip on touch devices", () => {
  it("exposes the full absolute time via a tap-to-open drawer instead of relying on hover-only title", () => {
    TOUCH_DRAWER.enabled = true;
    const expectedAbsoluteTime = new Date(MESSAGE_TIMESTAMP).toLocaleString();

    render(
      <StateProvider>
        <MessageActions message={assistantMessage()} />
      </StateProvider>,
    );

    const trigger = screen.getByTestId("message-timestamp-trigger");
    expect(trigger.querySelector("time")).not.toBeNull();
    expect(screen.queryByText(expectedAbsoluteTime)).toBeNull();

    fireEvent.click(trigger);

    expect(screen.getByText(expectedAbsoluteTime)).not.toBeNull();
  });
});
