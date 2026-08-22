import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type Turn,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { MessageActions } from "./message-actions";

const TOUCH_DRAWER = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => TOUCH_DRAWER.enabled,
}));

const MESSAGE_TIMESTAMP = "2026-07-20T10:15:00Z";
const MESSAGE_TURN_DURATION_TEST_ID = "message-turn-duration";

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

function userMessage(overrides: Partial<Message> = {}): Message {
  return {
    ...assistantMessage(),
    author_type: "user",
    turn_id: "turn-1",
    ...overrides,
  };
}

function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    id: "turn-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    created_at: MESSAGE_TIMESTAMP,
    started_at: MESSAGE_TIMESTAMP,
    updated_at: MESSAGE_TIMESTAMP,
    ...overrides,
  };
}

let storeApi: StoreApi<AppState> | null = null;

function StoreCapture() {
  storeApi = useAppStoreApi();
  return null;
}

function renderMessageActions(message: Message, messageTurn?: Turn) {
  render(
    <StateProvider>
      <StoreCapture />
      <MessageActions message={message} />
    </StateProvider>,
  );

  if (!messageTurn) return;

  const capturedStore = storeApi;
  if (!capturedStore) throw new Error("App store was not captured");
  act(() => {
    capturedStore.getState().addTurn(messageTurn);
  });
}

afterEach(() => {
  TOUCH_DRAWER.enabled = false;
  cleanup();
  storeApi = null;
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

describe("MessageActions action row disclosure", () => {
  it("keeps the action row visible for coarse pointers at tablet widths", () => {
    TOUCH_DRAWER.enabled = true;

    renderMessageActions(userMessage(), turn({ completed_at: "2026-07-20T10:15:05Z" }));

    const actions = screen.getByTestId(MESSAGE_TURN_DURATION_TEST_ID).parentElement;
    expect(actions).not.toBeNull();
    expect(actions?.className).toContain("opacity-100");
    expect(actions?.className).not.toContain("sm:opacity-0");
    expect(actions?.className).not.toContain("sm:group-hover:opacity-100");
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

describe("MessageActions turn duration", () => {
  it("renders the completed user prompt duration with an hourglass", () => {
    renderMessageActions(userMessage(), turn({ completed_at: "2026-07-20T10:15:05Z" }));

    const duration = screen.getByTestId(MESSAGE_TURN_DURATION_TEST_ID);
    expect(duration.textContent).toBe("5s");
    const hourglass = duration.querySelector("svg");
    expect(hourglass?.getAttribute("aria-hidden")).toBe("true");
    expect(hourglass?.getAttribute("class")).toContain("h-3");
    expect(hourglass?.getAttribute("class")).toContain("w-3");
  });

  it("omits duration while the prompt turn is running", () => {
    renderMessageActions(userMessage(), turn());

    expect(screen.queryByTestId(MESSAGE_TURN_DURATION_TEST_ID)).toBeNull();
  });

  it("renders a zero-second completed duration", () => {
    renderMessageActions(userMessage(), turn({ completed_at: "2026-07-20T10:14:59Z" }));

    expect(screen.getByTestId(MESSAGE_TURN_DURATION_TEST_ID).textContent).toBe("0s");
  });

  it("omits duration for completed agent messages", () => {
    renderMessageActions(
      assistantMessage({ turn_id: "turn-1" }),
      turn({ completed_at: "2026-07-20T10:15:05Z" }),
    );

    expect(screen.queryByTestId(MESSAGE_TURN_DURATION_TEST_ID)).toBeNull();
  });

  it("keeps a multi-unit duration unwrapped", () => {
    renderMessageActions(
      userMessage({ created_at: "2026-07-20T10:10:00Z" }),
      turn({ completed_at: "2026-07-20T10:15:23Z" }),
    );

    const duration = screen.getByTestId(MESSAGE_TURN_DURATION_TEST_ID);
    expect(duration.textContent).toBe("5m 23s");
    expect(duration.className).toContain("whitespace-nowrap");
    expect(duration.className).toContain("shrink-0");
  });
});
