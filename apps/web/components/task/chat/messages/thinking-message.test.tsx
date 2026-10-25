import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ThinkingMessage } from "./thinking-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

afterEach(cleanup);

const PREVIEW_TEST_ID = "thinking-message-preview";

function thinkingMessage(content: string): Message {
  return {
    id: "thinking-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "thinking",
    created_at: "2026-08-30T00:00:00Z",
    metadata: { thinking: content },
  };
}

describe("ThinkingMessage", () => {
  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.1, AC-UI-THINKING-MESSAGE-PREVIEW-001.2, AC-UI-THINKING-MESSAGE-PREVIEW-001.7
  it("shows the first meaningful markdown-stripped line in an expandable header", () => {
    render(
      <ThinkingMessage
        comment={thinkingMessage(
          "\n## \r\n**Choose the implementation path** [with context](https://example.test/context)\r\n\nLater details remain in full.",
        )}
      />,
    );

    const preview = screen.queryByText("Choose the implementation path with context", {
      exact: true,
    });

    expect(preview).not.toBeNull();
    expect(preview?.className).toContain("min-w-0");
    expect(preview?.className).toContain("truncate");
    expect(screen.queryByText("Later details remain in full.", { exact: true })).toBeNull();
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.2
  it("keeps the label-only fallback when decoration produces no visible text", () => {
    render(<ThinkingMessage comment={thinkingMessage("\n**\n## ")} />);

    expect(screen.getByText("Thinking", { exact: true })).toBeTruthy();
    expect(screen.queryByTestId(PREVIEW_TEST_ID)).toBeNull();
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.2
  it("skips fenced and thematic Markdown-only lines", () => {
    render(
      <ThinkingMessage
        comment={thinkingMessage("```\n---\n**First visible reasoning**\n\nLater detail.")}
      />,
    );

    expect(screen.getByTestId(PREVIEW_TEST_ID).textContent).toBe("First visible reasoning");
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.1
  it("preserves identifier punctuation in the preview", () => {
    render(<ThinkingMessage comment={thinkingMessage("foo_bar_baz\n\nLater detail.")} />);

    expect(screen.getByTestId(PREVIEW_TEST_ID).textContent).toBe("foo_bar_baz");
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.3
  it("keeps the original preview when later reasoning is appended", () => {
    const { rerender } = render(
      <ThinkingMessage comment={thinkingMessage("**Original subject**\n\nInitial detail.")} />,
    );

    rerender(
      <ThinkingMessage
        comment={thinkingMessage("**Original subject**\n\nInitial detail.\n\n**A later subject**")}
      />,
    );

    expect(screen.getByTestId(PREVIEW_TEST_ID).textContent).toBe("Original subject");
    expect(screen.queryByText("A later subject", { exact: true })).toBeNull();
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.5
  it("reveals the complete markdown content when expanded", () => {
    render(
      <ThinkingMessage
        comment={thinkingMessage("**Original subject**\n\nThe later details remain in full.")}
      />,
    );

    fireEvent.click(screen.getByText("Thinking", { exact: true }));

    expect(screen.getByText("The later details remain in full.", { exact: true })).toBeTruthy();
  });

  // @covers AC-UI-THINKING-MESSAGE-PREVIEW-001.6
  it("keeps compact single-line thinking inline and non-expandable", () => {
    const { container } = render(
      <ThinkingMessage comment={thinkingMessage("**Compact thought**")} />,
    );

    const compactText = screen.getByText("Compact thought", { exact: true });
    expect(compactText).toBeTruthy();
    expect(compactText.className).toContain("min-w-0");
    expect(compactText.className).toContain("break-words");
    expect(compactText.className).not.toContain("truncate");
    expect(screen.queryByTestId(PREVIEW_TEST_ID)).toBeNull();
    expect(container.querySelector(".markdown-body")).toBeNull();
  });
});
