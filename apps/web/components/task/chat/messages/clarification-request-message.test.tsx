import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationRequestMetadata,
  type Message,
} from "@/lib/types/http";
import { ClarificationRequestMessage } from "./clarification-request-message";

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
});
