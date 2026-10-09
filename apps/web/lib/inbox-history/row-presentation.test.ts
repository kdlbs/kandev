import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { Message } from "@/lib/types/http";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";
import {
  inboxHistoryClarificationQuestions,
  inboxHistoryPermissionContent,
  inboxHistoryReasonLabelKey,
  inboxHistorySecondaryText,
} from "./row-presentation";

const TASK_ID = "t1";
const SESSION_ID = "s1";
const CREATED_AT = "2026-09-03T04:54:02Z";

function bundle(overrides: Partial<InboxHistoryBundle> = {}): InboxHistoryBundle {
  return {
    pending_id: "p1",
    task_id: TASK_ID,
    session_id: SESSION_ID,
    session_state: "COMPLETED",
    task_title: "Task",
    created_at: CREATED_AT,
    context: "",
    messages: [],
    kind: "clarification",
    reason: "session_ended",
    asking_turn_id: "turn-1",
    ...overrides,
  };
}

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    author_type: "agent",
    content: "",
    type: "clarification_request",
    created_at: CREATED_AT,
    ...overrides,
  };
}

describe("inboxHistoryClarificationQuestions", () => {
  it("extracts title, prompt and option labels per question, in message order (AC .30, AC .19)", () => {
    const b = bundle({
      messages: [
        message({
          metadata: {
            question: {
              id: "q1",
              title: "Pick a branch",
              prompt: "Which branch should I use?",
              options: [
                { option_id: "o1", label: "main", description: "" },
                { option_id: "o2", label: "dev", description: "" },
              ],
            },
          },
        }),
      ],
    });

    const questions = inboxHistoryClarificationQuestions(b);

    expect(questions).toHaveLength(1);
    expect(questions[0]).toMatchObject({
      id: "q1",
      title: "Pick a branch",
      prompt: "Which branch should I use?",
      optionLabels: ["main", "dev"],
    });
  });

  it("says so rather than render blank when a question has no resolvable content (AC .13)", () => {
    const b = bundle({ messages: [message({ metadata: {} })] });

    const questions = inboxHistoryClarificationQuestions(b);

    expect(questions[0].title).toBe("");
    expect(questions[0].prompt).toBe("");
    expect(questions[0].id).toBe("p1-0");
  });
});

describe("inboxHistoryPermissionContent", () => {
  it("reads content and options[].name, never .label, for a permission bundle (AC .30)", () => {
    const b = bundle({
      kind: "permission",
      messages: [
        message({
          content: "Allow running `rm -rf tmp/`?",
          type: "permission_request",
          metadata: {
            options: [
              { kind: "allow", name: "Allow", option_id: "o1" },
              { kind: "reject", name: "Reject", option_id: "o2" },
            ],
          },
        }),
      ],
    });

    const result = inboxHistoryPermissionContent(b);

    expect(result.content).toBe("Allow running `rm -rf tmp/`?");
    expect(result.optionNames).toEqual(["Allow", "Reject"]);
  });
});

describe("inboxHistoryReasonLabelKey", () => {
  it("maps each of the three reasons to a distinct i18n key (AC .9)", () => {
    expect(inboxHistoryReasonLabelKey("superseded")).toBe("inboxHistory:reasonSuperseded");
    expect(inboxHistoryReasonLabelKey("session_ended")).toBe("inboxHistory:reasonSessionEnded");
    expect(inboxHistoryReasonLabelKey("unreadable")).toBe("inboxHistory:reasonUnreadable");
  });
});

describe("inboxHistorySecondaryText", () => {
  it("prefers task_title, falling back to task_id", () => {
    expect(inboxHistorySecondaryText(bundle({ task_title: "My task" }))).toBe("My task");
    expect(inboxHistorySecondaryText(bundle({ task_title: "", task_id: "t-42" }))).toBe("t-42");
  });
});
