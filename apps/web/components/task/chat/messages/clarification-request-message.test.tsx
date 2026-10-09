import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationRequestMetadata,
  type Message,
} from "@/lib/types/http";
import { ClarificationRequestMessage } from "./clarification-request-message";

const mockUpdateMessage = vi.hoisted(() => vi.fn());
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      updateMessage: mockUpdateMessage,
      messages: { bySession: {} },
    }),
  }),
}));

function answeredClarification(): Message {
  const metadata: ClarificationRequestMetadata = {
    pending_id: "pending-1",
    session_id: "session-1",
    status: "answered",
    question: {
      id: "question-1",
      title: "Deploy `now`",
      prompt: "Pick **one**:\n\n- Fast\n- Careful",
      options: [
        {
          option_id: "fast",
          label: "`Fast` path",
          description: "Best for **small** changes",
        },
      ],
    },
    response: {
      question_id: "question-1",
      selected_options: ["fast"],
      custom_text: "Keep `this` **literal**",
    },
  };

  return {
    id: "message-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "Question",
    type: "clarification_request",
    created_at: "2026-08-24T00:00:00Z",
    metadata,
  };
}

describe("ClarificationRequestMessage", () => {
  afterEach(cleanup);

  it("renders agent question Markdown but keeps custom answer text literal", () => {
    const { container } = render(<ClarificationRequestMessage comment={answeredClarification()} />);

    expect(Array.from(container.querySelectorAll("code"), (node) => node.textContent)).toEqual([
      "now",
      "Fast",
    ]);
    expect(container.querySelector("strong")?.textContent).toBe("one");
    expect(container.querySelector("ul")?.textContent).toContain("Careful");

    const customText = Array.from(container.querySelectorAll("span")).find(
      (node) => node.children.length === 0 && node.textContent?.includes("Keep `this` **literal**"),
    );
    expect(customText).toBeDefined();
    expect(customText?.querySelector("code, strong")).toBeNull();
  });

  it("offers an answer-as-new-message action for an unanswered historical question", () => {
    const message = answeredClarification();
    message.metadata = {
      ...(message.metadata as ClarificationRequestMetadata),
      status: "pending",
      response: undefined,
    };

    render(<ClarificationRequestMessage comment={message} />);

    expect(screen.getByTestId("clarification-answer-as-new-message")).toBeTruthy();
  });

  it("does not offer a second action for the current pending turn", () => {
    const message = answeredClarification();
    message.metadata = {
      ...(message.metadata as ClarificationRequestMetadata),
      status: "pending",
      response: undefined,
    };

    render(<ClarificationRequestMessage comment={message} isCurrentTurn />);

    expect(screen.queryByTestId("clarification-answer-as-new-message")).toBeNull();
  });

  it("sends a selected historical answer through the late-message callback", async () => {
    const onLateAnswer = vi.fn().mockResolvedValue("sent" as const);
    render(
      <ClarificationRequestMessage
        comment={{
          ...answeredClarification(),
          metadata: {
            ...(answeredClarification().metadata as ClarificationRequestMetadata),
            status: "expired",
            response: undefined,
          },
        }}
        onLateAnswer={onLateAnswer}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /answer as new message/i }));
    fireEvent.click(screen.getByTestId("clarification-option"));
    fireEvent.click(screen.getByTestId("clarification-late-submit"));

    await waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(1));
    expect(onLateAnswer.mock.calls[0][0].answers).toEqual([
      { question_id: "question-1", selected_options: ["fast"] },
    ]);
  });

  it("closes the historical form without admitting a message", () => {
    const onLateAnswer = vi.fn().mockResolvedValue("sent" as const);
    render(
      <ClarificationRequestMessage
        comment={{
          ...answeredClarification(),
          metadata: {
            ...(answeredClarification().metadata as ClarificationRequestMetadata),
            status: "expired",
            response: undefined,
          },
        }}
        onLateAnswer={onLateAnswer}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /answer as new message/i }));
    fireEvent.click(screen.getByTestId("clarification-late-close"));

    expect(onLateAnswer).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /answer as new message/i })).toBeTruthy();
  });
});
