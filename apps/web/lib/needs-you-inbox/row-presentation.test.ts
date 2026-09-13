import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { rowPrimaryText, rowSecondaryText } from "./row-presentation";

const FALLBACK = "fallback";
const QUESTION_FROM_AGENT = "Question from agent";
const QUESTION_PROMPT = "Which plan?";

function bundle(overrides: Partial<ClarificationInboxBundle> = {}): ClarificationInboxBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "WAITING_FOR_INPUT",
    task_title: "",
    created_at: "2026-09-03T04:54:02Z",
    context: "",
    messages: [],
    ...overrides,
  };
}

function messageWithQuestion(question: { title?: string; prompt?: string }) {
  return {
    id: "m1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent" as const,
    content: "",
    type: "clarification_request" as const,
    created_at: "2026-09-03T04:54:02Z",
    metadata: {
      pending_id: "p1",
      session_id: "s1",
      question: {
        id: "q1",
        title: question.title ?? "",
        prompt: question.prompt ?? "",
        options: [],
      },
    },
  };
}

describe("rowPrimaryText", () => {
  it("uses the first question's title when non-empty", () => {
    const b = bundle({ messages: [messageWithQuestion({ title: "Pick a plan" })] });
    expect(rowPrimaryText(b, FALLBACK)).toBe("Pick a plan");
  });

  it("falls back to the prompt when the title is empty", () => {
    const b = bundle({ messages: [messageWithQuestion({ prompt: QUESTION_PROMPT })] });
    expect(rowPrimaryText(b, FALLBACK)).toBe(QUESTION_PROMPT);
  });

  it("falls back to the prompt when the title contains only whitespace", () => {
    const b = bundle({
      messages: [messageWithQuestion({ title: "  \n", prompt: QUESTION_PROMPT })],
    });
    expect(rowPrimaryText(b, FALLBACK)).toBe(QUESTION_PROMPT);
  });

  it("falls back to the bundle context when title and prompt are both empty", () => {
    const b = bundle({ context: "Deploying to prod", messages: [messageWithQuestion({})] });
    expect(rowPrimaryText(b, FALLBACK)).toBe("Deploying to prod");
  });

  it("falls back to the localized default when nothing else is available", () => {
    const b = bundle({ messages: [messageWithQuestion({})] });
    expect(rowPrimaryText(b, QUESTION_FROM_AGENT)).toBe(QUESTION_FROM_AGENT);
  });

  it("falls back to the default when there are no messages at all", () => {
    const b = bundle({ messages: [] });
    expect(rowPrimaryText(b, QUESTION_FROM_AGENT)).toBe(QUESTION_FROM_AGENT);
  });
});

describe("rowSecondaryText", () => {
  it("prefers the task title", () => {
    expect(rowSecondaryText(bundle({ task_title: "Fix the build", task_id: "t1" }))).toBe(
      "Fix the build",
    );
  });

  it("falls back to the task id when the title is empty", () => {
    expect(rowSecondaryText(bundle({ task_title: "", task_id: "t1" }))).toBe("t1");
  });
});
