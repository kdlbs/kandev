import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import { MessageActions } from "./message-actions";

afterEach(cleanup);

const message: Message = {
  id: "message-1",
  session_id: sessionId("session-1"),
  task_id: taskId("task-1"),
  author_type: "user",
  content: "Keep the remaining actions",
  type: "message",
  created_at: "2026-07-21T00:00:00Z",
};

describe("MessageActions", () => {
  it("renders user navigation alongside the existing actions", () => {
    const onToggleRaw = vi.fn();
    const onPrevious = vi.fn();
    const onNext = vi.fn();
    render(
      <StateProvider>
        <MessageActions
          message={message}
          onToggleRaw={onToggleRaw}
          navigation={{
            canNavigatePrevious: true,
            canNavigateNext: false,
            isBusy: false,
            onPrevious,
            onNext,
          }}
        />
      </StateProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show raw text" }));
    const previous = screen.getByRole("button", { name: "Previous user message" });
    previous.focus();
    fireEvent.click(previous);

    expect(onToggleRaw).toHaveBeenCalledOnce();
    expect(onPrevious).toHaveBeenCalledOnce();
    expect(onNext).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(previous);
    expect(screen.getByRole("button", { name: "Copy message to clipboard" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Next user message" }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("does not render navigation for an agent message", () => {
    render(
      <StateProvider>
        <MessageActions
          message={{ ...message, author_type: "agent" }}
          navigation={{
            canNavigatePrevious: true,
            canNavigateNext: true,
            isBusy: false,
            onPrevious: vi.fn(),
            onNext: vi.fn(),
          }}
        />
      </StateProvider>,
    );

    expect(screen.queryByRole("button", { name: "Previous user message" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Next user message" })).toBeNull();
  });
});
