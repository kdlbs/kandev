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
const SUBAGENT_DESCRIPTION = "subagent-description";
const SUBAGENT_RESULT_TEXT = "subagent-result-text";
const CHILD_TOOL_LABEL = "Read SyncRunner.ts";
const CODE_REVIEWER = "code-reviewer";

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

describe("isSubagentEffectivelyActive", () => {
  it.each<{
    name: string;
    metadataStatus: ToolCallMetadata["status"];
    payloadStatus: string;
    isContainingTurnActive: boolean;
    expected: boolean;
  }>([
    {
      name: "in-progress metadata is active during its turn without a started payload",
      metadataStatus: IN_PROGRESS,
      payloadStatus: "queued",
      isContainingTurnActive: true,
      expected: true,
    },
    {
      name: "in-progress metadata settles with its turn without a started payload",
      metadataStatus: IN_PROGRESS,
      payloadStatus: "queued",
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
      payloadStatus: "queued",
      isContainingTurnActive: false,
      expected: true,
    },
    {
      name: "terminal metadata overrides a started payload in an active turn",
      metadataStatus: COMPLETE,
      payloadStatus: STARTED,
      isContainingTurnActive: true,
      expected: false,
    },
  ])("$name", ({ metadataStatus, payloadStatus, isContainingTurnActive, expected }) => {
    const message = subagentMessage({ metadataStatus, payloadStatus });
    const metadata = message.metadata as ToolCallMetadata;
    if (metadataStatus === undefined) delete metadata.status;

    expect(isSubagentEffectivelyActive(metadata, isContainingTurnActive)).toBe(expected);
  });
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
});

describe("ToolSubagentMessage expansion", () => {
  it("expands nested child tools and keeps their count", () => {
    const childMessages = [
      childTool("child-1", "first child"),
      childTool("child-2", "second child"),
    ];
    renderSubagent(subagentMessage({ metadataStatus: COMPLETE, payloadStatus: COMPLETE }), {
      childMessages,
    });

    expect(screen.getByTestId("subagent-child-count").textContent).toBe("2 tool calls");
    expect(screen.getByTestId(SUBAGENT_CHEVRON)).toBeTruthy();
    expect(screen.queryByText("first child")).toBeNull();

    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("first child")).toBeTruthy();
    expect(screen.getByText("second child")).toBeTruthy();
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
