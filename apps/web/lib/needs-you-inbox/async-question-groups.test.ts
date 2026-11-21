import { describe, expect, it } from "vitest";
import { asyncQuestionPrimaryText, groupAsyncQuestionsByTask } from "./async-question-groups";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

function question(
  overrides: Partial<ClarificationInboxAsyncQuestion> = {},
): ClarificationInboxAsyncQuestion {
  return {
    question_id: "q1",
    task_id: "t1",
    session_id: "s1",
    session_state: "RUNNING",
    task_title: "Checkout integration",
    forwarded: false,
    waiting: false,
    status: "pending",
    context: "",
    created_at: "2026-10-30T10:00:00Z",
    questions: [
      {
        id: "q1",
        title: "Database choice",
        prompt: "Which database should the service use?",
        options: [
          { option_id: "a", label: "Postgres", description: "Managed" },
          { option_id: "b", label: "SQLite", description: "Embedded" },
        ],
      },
    ],
    ...overrides,
  };
}

describe("groupAsyncQuestionsByTask", () => {
  it("groups requests by task while counting requests, not groups", () => {
    const groups = groupAsyncQuestionsByTask([
      question({ question_id: "q1" }),
      question({ question_id: "q2", task_id: "t2", task_title: "Copy update" }),
      question({ question_id: "q3" }),
    ]);
    expect(groups).toHaveLength(2);
    expect(groups[0].taskId).toBe("t1");
    expect(groups[0].questions.map((item) => item.question_id)).toEqual(["q1", "q3"]);
    expect(groups[1].taskTitle).toBe("Copy update");
  });

  it("preserves server order for an empty population", () => {
    expect(groupAsyncQuestionsByTask([])).toEqual([]);
  });
});

describe("asyncQuestionPrimaryText", () => {
  it("prefers the short title, then the prompt", () => {
    expect(asyncQuestionPrimaryText(question())).toBe("Database choice");
    const withoutTitle = question();
    withoutTitle.questions[0].title = "";
    expect(asyncQuestionPrimaryText(withoutTitle)).toBe("Which database should the service use?");
  });
});
