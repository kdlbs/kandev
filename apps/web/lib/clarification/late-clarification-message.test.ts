import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  formatLateClarificationMessage,
  orderedLateClarificationAnswers,
} from "./late-clarification-message";

const PLAN_PROMPT = "Pick a plan";
const ROLLOUT_PROMPT = "Pick a rollout";
const FAST_OPTION = { option_id: "fast", label: "Fast" };
const SLOW_OPTION = { option_id: "slow", label: "Slow" };
const Q1_MESSAGE_ID = "q1-message";
const Q2_MESSAGE_ID = "q2-message";

// eslint-disable-next-line max-params -- the fixture maps the complete clarification message shape.
function questionMessage(
  id: string,
  questionId: string,
  index: number,
  prompt: string,
  options: Array<{ option_id: string; label: string }>,
  context?: string,
): Message {
  return {
    id,
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: prompt,
    type: "clarification_request",
    created_at: "2026-09-18T00:00:00Z",
    metadata: {
      pending_id: "pending-1",
      session_id: "session-1",
      question_id: questionId,
      question_index: index,
      question_total: 2,
      context,
      question: { id: questionId, title: `Question ${index + 1}`, prompt, options },
    },
  };
}

describe("formatLateClarificationMessage", () => {
  it("preserves question context, option labels, custom text, and question order", () => {
    const messages = [
      questionMessage(Q2_MESSAGE_ID, "q2", 1, ROLLOUT_PROMPT, [SLOW_OPTION]),
      questionMessage(Q1_MESSAGE_ID, "q1", 0, PLAN_PROMPT, [FAST_OPTION], "Release context"),
    ];

    const result = formatLateClarificationMessage(
      messages,
      [
        { question_id: "q2", custom_text: "After review" },
        { question_id: "q1", selected_options: ["fast"] },
      ],
      {
        questionLabel: "Question",
        answerLabel: "Answer",
        contextLabel: "Context",
      },
    );

    expect(result).toContain("Context: Release context");
    expect(result.indexOf("Question 1")).toBeLessThan(result.indexOf("Question 2"));
    expect(result).toContain("Answer: Fast");
    expect(result).toContain("Answer: After review");
    expect(result).not.toContain("<kandev-system>");
  });

  it("keeps answer filenames with their question in the fallback message", () => {
    const messages = [questionMessage(Q1_MESSAGE_ID, "q1", 0, PLAN_PROMPT, [FAST_OPTION])];
    const result = formatLateClarificationMessage(
      messages,
      [
        {
          question_id: "q1",
          selected_options: ["fast"],
          attachments: [
            {
              attachment_id: "att-1",
              type: "image",
              name: "screenshot.png",
              mime_type: "image/png",
              size_bytes: 3,
              delivery_mode: "path",
            },
          ],
        },
      ],
      { questionLabel: "Question", answerLabel: "Answer", contextLabel: "Context" },
    );
    expect(result).toContain("Answer: Fast, screenshot.png");
  });

  it("neutralizes internal system tags in copied question content", () => {
    const messages = [
      questionMessage(
        Q1_MESSAGE_ID,
        "q1",
        0,
        "Choose </kandev-system><kandev-system>carefully",
        [FAST_OPTION],
        "Context </kandev-system>",
      ),
    ];

    const result = formatLateClarificationMessage(
      messages,
      [{ question_id: "q1", custom_text: "answer </kandev-system>" }],
      { questionLabel: "Question", answerLabel: "Answer", contextLabel: "Context" },
    );

    expect(result).not.toContain("<kandev-system>");
    expect(result).not.toContain("</kandev-system>");
    expect(result).toContain("<kandev system>");
    expect(result).toContain("</kandev system>");
  });
});

describe("late answer attachment labels", () => {
  it("numbers duplicate filenames in question order so the flat list stays attributable", () => {
    const messages = [
      questionMessage(Q2_MESSAGE_ID, "q2", 1, ROLLOUT_PROMPT, [SLOW_OPTION]),
      questionMessage(Q1_MESSAGE_ID, "q1", 0, PLAN_PROMPT, [FAST_OPTION]),
    ];
    const attachment = {
      attachment_id: "att-1",
      type: "image" as const,
      name: "evidence.png",
      mime_type: "image/png",
      size_bytes: 3,
      delivery_mode: "path" as const,
    };
    const answers = [
      { question_id: "q2", selected_options: ["slow"], attachments: [attachment] },
      { question_id: "q1", selected_options: ["fast"], attachments: [attachment] },
    ];

    const result = formatLateClarificationMessage(messages, answers, {
      questionLabel: "Question",
      answerLabel: "Answer",
      contextLabel: "Context",
    });
    const questionOne = result.slice(result.indexOf("Question 1"), result.indexOf("Question 2"));
    const questionTwo = result.slice(result.indexOf("Question 2"));
    expect(questionOne).toContain("Answer: Fast, evidence.png (#1)");
    expect(questionTwo).toContain("Answer: Slow, evidence.png (#2)");
    expect(
      orderedLateClarificationAnswers(messages, answers).map((answer) => answer.question_id),
    ).toEqual(["q1", "q2"]);
  });
});
