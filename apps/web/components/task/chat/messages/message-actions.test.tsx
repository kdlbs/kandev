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
  it("keeps message actions without rendering per-message navigation", () => {
    const onToggleRaw = vi.fn();
    render(
      <StateProvider>
        <MessageActions message={message} onToggleRaw={onToggleRaw} />
      </StateProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show raw text" }));

    expect(onToggleRaw).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Copy message to clipboard" })).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Go to previous message" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Go to next message" })).toBeNull();
  });
});
