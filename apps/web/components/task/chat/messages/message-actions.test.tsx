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

describe("MessageActions favorite toggle", () => {
  it("renders a star button reflecting isFavorite and calls onToggleFavorite when clicked", () => {
    const onToggleFavorite = vi.fn();
    render(
      <StateProvider>
        <MessageActions
          message={assistantMessage()}
          isFavorite={false}
          onToggleFavorite={onToggleFavorite}
        />
      </StateProvider>,
    );

    const star = screen.getByRole("button", { name: /mark message as favorite/i });
    expect(star.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(star);

    expect(onToggleFavorite).toHaveBeenCalledTimes(1);

    // The favorite control must match the sibling action buttons' shared
    // sizing on every viewport instead of the oversized mobile 44px target.
    expect(star.className).toContain("h-5 w-5 p-1");
    expect(star.className).not.toMatch(/\bmin-h-11\b/);
    expect(star.className).not.toMatch(/\bmin-w-11\b/);

    const icon = star.querySelector("svg");
    expect(icon?.getAttribute("class")).toContain("h-full");
    expect(icon?.getAttribute("class")).toContain("w-full");
  });

  it("shows a filled star and 'remove from favorites' label when isFavorite is true", () => {
    render(
      <StateProvider>
        <MessageActions
          message={assistantMessage()}
          isFavorite={true}
          onToggleFavorite={() => {}}
        />
      </StateProvider>,
    );

    const star = screen.getByRole("button", { name: /remove message from favorites/i });
    expect(star.getAttribute("aria-pressed")).toBe("true");
  });

  it("omits the favorite button entirely when onToggleFavorite is not provided", () => {
    render(
      <StateProvider>
        <MessageActions message={assistantMessage()} />
      </StateProvider>,
    );

    expect(screen.queryByRole("button", { name: /favorite/i })).toBeNull();
  });
});
