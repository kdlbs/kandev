import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { isSubagentEffectivelyActive, ToolSubagentMessage } from "./tool-subagent-message";

afterEach(cleanup);

const COMPLETE = "complete";
const SUBAGENT_CHEVRON = "subagent-chevron";
const IN_PROGRESS = "in_progress";
const STARTED = "started";
const WORKING = "Working...";
const COMPLETED = "Completed";
const FAILED = "Failed";
const CANCELLED = "Cancelled";
const SUBAGENT_DESCRIPTION = "subagent-description";
const SUBAGENT_RESULT_TEXT = "subagent-result-text";
const CHILD_TOOL_LABEL = "Read SyncRunner.ts";
const CODE_REVIEWER = "code-reviewer";
const FIRST_CHILD = "first child";
const SECOND_CHILD = "second child";
const QUEUED = "queued";

function subagentMessage({
  metadataStatus = "in_progress",
  payloadStatus = "started",
  description = "ten_second_probe",
  subagentType = "subagent",
  prompt,
  resultText,
  durationMs,
}: {
  metadataStatus?: ToolCallMetadata["status"];
  payloadStatus?: string;
  description?: string;
  subagentType?: string;
  prompt?: string;
  resultText?: string;
  durationMs?: number;
} = {}): Message {
  return {
    id: "codex-subagent-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "tool_call",
    content: "ten_second_probe",
    created_at: "2026-07-23T12:00:00Z",
    metadata: {
      status: metadataStatus,
      tool_call_id: "codex-subagent-tool-1",
      normalized: {
        kind: "subagent_task",
        subagent_task: {
          description,
          subagent_type: subagentType,
          status: payloadStatus,
          child_session_id: "child-session-123456",
          prompt,
          result_text: resultText,
          duration_ms: durationMs,
        },
      },
    },
  };
}

function childTool(id: string, content: string): Message {
  return {
    id,
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "tool_call",
    content,
    created_at: "2026-07-23T12:00:01Z",
    metadata: { status: "complete", tool_call_id: id },
  };
}

function renderSubagent(
  comment: Message,
  {
    childMessages = [],
    isContainingTurnActive = false,
  }: { childMessages?: Message[]; isContainingTurnActive?: boolean } = {},
) {
  return render(
    <ToolSubagentMessage
      comment={comment}
      childMessages={childMessages}
      isContainingTurnActive={isContainingTurnActive}
      renderChild={(message) => <span>{message.content}</span>}
    />,
  );
}

const EFFECTIVELY_ACTIVE_CASES: Array<{
  name: string;
  metadataStatus: ToolCallMetadata["status"];
  payloadStatus: string;
  isContainingTurnActive: boolean;
  expected: boolean;
}> = [
  {
    name: "in-progress metadata is active during its turn without a started payload",
    metadataStatus: IN_PROGRESS,
    payloadStatus: QUEUED,
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "in-progress metadata settles with its turn without a started payload",
    metadataStatus: IN_PROGRESS,
    payloadStatus: QUEUED,
    isContainingTurnActive: false,
    expected: false,
  },
  {
    name: "started payload with pending metadata is active during its turn",
    metadataStatus: "pending",
    payloadStatus: STARTED,
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "started payload with pending metadata settles with its turn",
    metadataStatus: "pending",
    payloadStatus: STARTED,
    isContainingTurnActive: false,
    expected: false,
  },
  {
    name: "started payload without metadata status is active during its turn",
    metadataStatus: undefined,
    payloadStatus: STARTED,
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "started payload without metadata status settles with its turn",
    metadataStatus: undefined,
    payloadStatus: STARTED,
    isContainingTurnActive: false,
    expected: false,
  },
  {
    name: "running metadata stays active without a containing-turn signal",
    metadataStatus: "running",
    payloadStatus: QUEUED,
    isContainingTurnActive: false,
    expected: true,
  },
  {
    name: "terminal nested payload overrides running metadata",
    metadataStatus: "running",
    payloadStatus: "errored",
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "started payload stays active after spawn completes during its turn",
    metadataStatus: COMPLETE,
    payloadStatus: STARTED,
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "pendingInit payload stays active after spawn completes during its turn",
    metadataStatus: COMPLETE,
    payloadStatus: "pendingInit",
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "pendingInit payload settles with its turn after spawn completes",
    metadataStatus: COMPLETE,
    payloadStatus: "pendingInit",
    isContainingTurnActive: false,
    expected: false,
  },
  {
    name: "terminal nested payload stays settled after spawn completes",
    metadataStatus: COMPLETE,
    payloadStatus: COMPLETE,
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "async_launched payload stays active after spawn completes during its turn",
    metadataStatus: COMPLETE,
    payloadStatus: "async_launched",
    isContainingTurnActive: true,
    expected: true,
  },
  {
    name: "async_launched payload settles with its turn after spawn completes",
    metadataStatus: COMPLETE,
    payloadStatus: "async_launched",
    isContainingTurnActive: false,
    expected: false,
  },
  {
    name: "ACP completed nested payload settles after spawn completes",
    metadataStatus: COMPLETE,
    payloadStatus: "completed",
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "failed metadata stays settled during an active turn",
    metadataStatus: "failed",
    payloadStatus: STARTED,
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "cancelled metadata stays settled during an active turn",
    metadataStatus: "cancelled",
    payloadStatus: STARTED,
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "errored nested payload settles even while the turn is active",
    metadataStatus: "complete",
    payloadStatus: "errored",
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "interrupted nested payload settles even while the turn is active",
    metadataStatus: "complete",
    payloadStatus: "interrupted",
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "shutdown nested payload settles even while the turn is active",
    metadataStatus: "complete",
    payloadStatus: "shutdown",
    isContainingTurnActive: true,
    expected: false,
  },
  {
    name: "notFound nested payload settles even while the turn is active",
    metadataStatus: "complete",
    payloadStatus: "notFound",
    isContainingTurnActive: true,
    expected: false,
  },
];

describe("isSubagentEffectivelyActive", () => {
  it.each(EFFECTIVELY_ACTIVE_CASES)(
    "$name",
    ({ metadataStatus, payloadStatus, isContainingTurnActive, expected }) => {
      const message = subagentMessage({ metadataStatus, payloadStatus });
      const metadata = message.metadata as ToolCallMetadata;
      if (metadataStatus === undefined) delete metadata.status;

      expect(isSubagentEffectivelyActive(metadata, isContainingTurnActive)).toBe(expected);
    },
  );
});

describe("ToolSubagentMessage", () => {
  it("does not expose a toggle for a settled contentless Codex subagent", () => {
    renderSubagent(subagentMessage());

    expect(screen.getByTestId("subagent-type").textContent).toContain("subagent");
    expect(screen.getByTestId("subagent-meta-session")).toBeTruthy();
    expect(screen.queryByTestId(SUBAGENT_CHEVRON)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();

    fireEvent.click(screen.getByTestId("subagent-header"));
    expect(screen.queryByText(WORKING)).toBeNull();
  });

  it("shows a contentless active subagent as one non-expandable status row", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: "running",
        description: "verify",
        subagentType: "verify",
      }),
    );

    expect(screen.getByTestId("subagent-type").textContent).toBe("verify");
    expect(screen.queryByTestId(SUBAGENT_DESCRIPTION)).toBeNull();
    expect(screen.getByText(WORKING)).toBeTruthy();
    expect(screen.queryByTestId(SUBAGENT_CHEVRON)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("shows work only while a stale Codex lifecycle is in an active turn", () => {
    const comment = subagentMessage();
    const { rerender } = renderSubagent(comment, { isContainingTurnActive: true });

    expect(screen.getByRole("status", { name: "Loading" })).toBeTruthy();
    expect(screen.getByText(WORKING)).toBeTruthy();

    rerender(
      <ToolSubagentMessage
        comment={comment}
        childMessages={[]}
        isContainingTurnActive={false}
        renderChild={(message) => <span>{message.content}</span>}
      />,
    );

    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(screen.queryByText(WORKING)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("shows Working after spawn completes while the nested Codex task is still pendingInit", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: "pendingInit" }), {
      isContainingTurnActive: true,
    });

    expect(screen.getByRole("status", { name: "Loading" })).toBeTruthy();
    expect(screen.getByText(WORKING)).toBeTruthy();
    expect(screen.getByTitle("Working")).toBeTruthy();
  });

  it("shows Working after spawn completes while the nested Claude task is still async_launched", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: "async_launched" }), {
      isContainingTurnActive: true,
    });

    expect(screen.getByRole("status", { name: "Loading" })).toBeTruthy();
    expect(screen.getByText(WORKING)).toBeTruthy();
    expect(screen.getByTitle("Working")).toBeTruthy();
  });

  it("does not keep Working after a nested Codex task settles as errored", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: "errored" }), {
      isContainingTurnActive: true,
    });

    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(screen.queryByText(WORKING)).toBeNull();
    expect(screen.getByLabelText(FAILED)).toBeTruthy();
  });

  it("shows a failed nested payload when spawn metadata is complete", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: "failed" }), {
      isContainingTurnActive: true,
    });

    expect(screen.getByLabelText(FAILED)).toBeTruthy();
    expect(screen.queryByLabelText(COMPLETED)).toBeNull();
  });

  it("shows a cancelled nested payload when spawn metadata is complete", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: "cancelled" }), {
      isContainingTurnActive: true,
    });

    expect(screen.getByLabelText(CANCELLED)).toBeTruthy();
    expect(screen.queryByLabelText(COMPLETED)).toBeNull();
  });

  it("shows a completed check with a mapped hover after the turn settles", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE }));

    expect(screen.getByTitle(COMPLETED)).toBeTruthy();
    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(screen.queryByText(WORKING)).toBeNull();
  });

  it("names a failed status on hover", () => {
    renderSubagent(subagentMessage({ metadataStatus: "failed", payloadStatus: COMPLETE }));

    expect(screen.getByTitle(FAILED)).toBeTruthy();
    expect(screen.queryByText(WORKING)).toBeNull();
  });

  it("names a cancelled status on hover", () => {
    renderSubagent(subagentMessage({ metadataStatus: "cancelled", payloadStatus: COMPLETE }));

    expect(screen.getByTitle(CANCELLED)).toBeTruthy();
    expect(screen.queryByText(WORKING)).toBeNull();
  });
});

describe("ToolSubagentMessage expansion", () => {
  it("expands nested child tools and keeps their count", () => {
    const childMessages = [childTool("child-1", FIRST_CHILD), childTool("child-2", SECOND_CHILD)];
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE }), {
      childMessages,
    });

    expect(screen.getByTestId("subagent-child-count").textContent).toBe("2 tool calls");
    expect(screen.getByTestId(SUBAGENT_CHEVRON)).toBeTruthy();
    expect(screen.queryByText(FIRST_CHILD)).toBeNull();

    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText(FIRST_CHILD)).toBeTruthy();
    expect(screen.getByText(SECOND_CHILD)).toBeTruthy();
  });

  it("keeps completed result-only subagents collapsed but expandable", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        resultText: "Probe completed successfully",
      }),
    );

    expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByTestId(SUBAGENT_RESULT_TEXT)).toBeNull();

    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByTestId(SUBAGENT_RESULT_TEXT).textContent).toBe(
      "Probe completed successfully",
    );
  });

  it("keeps a completed result collapsed after a contentless active state", () => {
    const activeComment = subagentMessage({
      metadataStatus: "running",
      payloadStatus: STARTED,
    });
    const { rerender } = renderSubagent(activeComment);

    expect(screen.getByText(WORKING)).toBeTruthy();

    rerender(
      <ToolSubagentMessage
        comment={subagentMessage({
          metadataStatus: COMPLETE,
          payloadStatus: COMPLETE,
          resultText: "Final summary",
        })}
        childMessages={[]}
        isContainingTurnActive={false}
        renderChild={(message) => <span>{message.content}</span>}
      />,
    );

    expect(screen.queryByTestId(SUBAGENT_RESULT_TEXT)).toBeNull();
  });

  it("keeps prompt-only subagents expandable", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        prompt: "Inspect the lifecycle events",
      }),
    );

    expect(screen.getByTestId(SUBAGENT_CHEVRON)).toBeTruthy();
    expect(screen.queryByText("Inspect the lifecycle events")).toBeNull();

    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("Inspect the lifecycle events")).toBeTruthy();
  });

  it("renders a completed contentless card as settled metadata", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        durationMs: 2500,
      }),
    );

    expect(screen.getByTestId("subagent-meta-session")).toBeTruthy();
    expect(screen.getByTestId("subagent-meta-duration").textContent).toBe("2.5s");
    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(screen.queryByTestId(SUBAGENT_CHEVRON)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("subagent header status", () => {
  const LOADING = { name: "Loading" } as const;

  it("keeps Working and the spinner on an active card that already has children", () => {
    renderSubagent(subagentMessage({ metadataStatus: "running", payloadStatus: STARTED }), {
      childMessages: [childTool("child-1", FIRST_CHILD), childTool("child-2", SECOND_CHILD)],
    });

    expect(screen.getByText(WORKING)).toBeTruthy();
    expect(screen.getByRole("status", LOADING)).toBeTruthy();
  });

  it("shows a completed check without Working or a spinner on a completed card with children", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE }), {
      childMessages: [childTool("child-1", FIRST_CHILD), childTool("child-2", SECOND_CHILD)],
    });

    expect(screen.queryByText(WORKING)).toBeNull();
    expect(screen.queryByRole("status", LOADING)).toBeNull();
    expect(screen.queryByLabelText("Failed")).toBeNull();
    expect(screen.queryByLabelText("Cancelled")).toBeNull();
    expect(screen.getByLabelText(COMPLETED)).toBeTruthy();
  });

  it("marks a failed card with a Failed label and no spinner", () => {
    renderSubagent(subagentMessage({ metadataStatus: "failed", payloadStatus: COMPLETE }));

    expect(screen.getByLabelText("Failed")).toBeTruthy();
    expect(screen.queryByRole("status", LOADING)).toBeNull();
    expect(screen.queryByText(WORKING)).toBeNull();
  });

  it("marks a cancelled card with a Cancelled label and no spinner", () => {
    renderSubagent(subagentMessage({ metadataStatus: "cancelled", payloadStatus: COMPLETE }));

    expect(screen.getByLabelText("Cancelled")).toBeTruthy();
    expect(screen.queryByRole("status", LOADING)).toBeNull();
    expect(screen.queryByText(WORKING)).toBeNull();
  });
});

// A reviewer's verdict is the only thing anyone reads a review-wave card for.
// It arrives on `toolResponse.content` and, before this, was shown only when
// the subagent streamed no child tool calls — i.e. never, for Claude.
describe("subagent result summary", () => {
  const VERDICT = "VERDICT: REQUEST_CHANGES\nTwo blocking findings in SyncRunner.ts.";

  it("shows a one-line summary on the collapsed card even when children exist", () => {
    renderSubagent(
      subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE, resultText: VERDICT }),
      { childMessages: [childTool("c1", CHILD_TOOL_LABEL)] },
    );
    const summary = screen.getByTestId("subagent-result-summary");
    expect(summary.textContent).toBe("VERDICT: REQUEST_CHANGES");
    expect(summary.textContent).not.toContain("Two blocking findings");
  });

  it("shows the full result above the children once expanded", () => {
    renderSubagent(
      subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE, resultText: VERDICT }),
      { childMessages: [childTool("c1", CHILD_TOOL_LABEL)] },
    );
    fireEvent.click(screen.getByTestId(SUBAGENT_CHEVRON).closest("button")!);
    expect(screen.getByTestId(SUBAGENT_RESULT_TEXT).textContent).toBe(VERDICT);
    expect(screen.getByText(CHILD_TOOL_LABEL)).toBeTruthy();
  });

  it("stays silent when no result was captured", () => {
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE }), {
      childMessages: [childTool("c1", CHILD_TOOL_LABEL)],
    });
    expect(screen.queryByTestId("subagent-result-summary")).toBeNull();
  });

  it("skips leading blank lines when picking the summary line", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        resultText: "\n\n  APPROVE  \nrest",
      }),
    );
    expect(screen.getByTestId("subagent-result-summary").textContent).toBe("APPROVE");
  });
});

// The type chip already says TEST-SUPERVISOR; repeating it as the first word of
// the description spent a third of a truncated line on a word already on screen.
describe("subagent description de-duplication", () => {
  it("strips a leading restatement of the subagent type", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        subagentType: "test-supervisor",
        description: "Test-supervisor review of new invariant tests",
      }),
    );
    expect(screen.getByTestId(SUBAGENT_DESCRIPTION).textContent).toBe(
      "review of new invariant tests",
    );
  });

  it("keeps a description that merely starts with a similar word", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        subagentType: CODE_REVIEWER,
        description: "code-review of the closure diff",
      }),
    );
    expect(screen.getByTestId(SUBAGENT_DESCRIPTION).textContent).toBe(
      "code-review of the closure diff",
    );
  });

  it("renders no description when it exactly restates the type", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        subagentType: CODE_REVIEWER,
        description: CODE_REVIEWER,
      }),
    );
    expect(screen.queryByTestId(SUBAGENT_DESCRIPTION)).toBeNull();
  });
});

// A description opening with a filename must not be eaten by a type that is a
// prefix of it: type "test" + "test.ts regression" must not become "ts
// regression". Only whitespace and a colon separate a type from its description.
describe("subagent description prefix boundaries", () => {
  it.each([
    ["test", "test.ts regression suite", "test.ts regression suite"],
    ["review", "review.md findings", "review.md findings"],
    [CODE_REVIEWER, "code-reviewer: the closure diff", "the closure diff"],
    [CODE_REVIEWER, "code-reviewer on diff", "on diff"],
  ])("type %s + %s renders %s", (subagentType, description, expected) => {
    cleanup();
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        subagentType,
        description,
      }),
    );
    expect(screen.getByTestId(SUBAGENT_DESCRIPTION).textContent).toBe(expected);
  });
});
