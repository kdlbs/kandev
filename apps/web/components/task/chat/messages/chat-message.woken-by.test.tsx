import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { sessionId, taskId, type Message } from "@/lib/types/http";

vi.mock("./woken-by-entry", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./woken-by-entry")>();
  return {
    ...actual,
    WokenByEntry: (props: { turnId: string }) => (
      <div data-testid="woken-by-stub">{props.turnId}</div>
    ),
  };
});

import { ChatMessage } from "./chat-message";

afterEach(cleanup);

function message(metadata?: Record<string, unknown>): Message {
  return {
    id: "m1",
    session_id: sessionId("s1"),
    task_id: taskId("t1"),
    author_type: "user",
    content: "hello there",
    type: "message",
    created_at: "2026-09-30T10:00:00Z",
    metadata,
  } as Message;
}

function renderMessage(comment: Message) {
  return render(
    <StateProvider>
      <ToastProvider>
        <ChatMessage comment={comment} label="User" className="" />
      </ToastProvider>
    </StateProvider>,
  );
}

describe("ChatMessage wake-turn routing", () => {
  it("routes a message carrying coordinator_wake_turn_id to the Woken by entry", () => {
    renderMessage(message({ coordinator_wake_turn_id: "turn-9" }));
    expect(screen.getByTestId("woken-by-stub").textContent).toBe("turn-9");
  });

  it.each([{}, { coordinator_wake_turn_id: "" }, { coordinator_wake_turn_id: 4 }, undefined])(
    "renders %j as an ordinary message",
    (metadata) => {
      renderMessage(message(metadata as Record<string, unknown> | undefined));
      expect(screen.queryByTestId("woken-by-stub")).toBeNull();
      expect(screen.getByText("hello there")).toBeTruthy();
    },
  );
});
