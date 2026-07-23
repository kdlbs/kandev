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

function subagentMessage({
  metadataStatus = "in_progress",
  payloadStatus = "started",
  prompt,
  resultText,
  durationMs,
}: {
  metadataStatus?: ToolCallMetadata["status"];
  payloadStatus?: string;
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
          description: "ten_second_probe",
          subagent_type: "subagent",
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
    expect(screen.queryByText("Subagent working...")).toBeNull();
  });

  it("shows work only while a stale Codex lifecycle is in an active turn", () => {
    const comment = subagentMessage();
    const { rerender } = renderSubagent(comment, { isContainingTurnActive: true });

    expect(screen.getByRole("status", { name: "Loading" })).toBeTruthy();
    expect(screen.getByText("Subagent working...")).toBeTruthy();

    rerender(
      <ToolSubagentMessage
        comment={comment}
        childMessages={[]}
        isContainingTurnActive={false}
        renderChild={(message) => <span>{message.content}</span>}
      />,
    );

    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(screen.queryByText("Subagent working...")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

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

  it("keeps result-only subagents expandable and auto-expanded", () => {
    renderSubagent(
      subagentMessage({
        metadataStatus: COMPLETE,
        payloadStatus: COMPLETE,
        resultText: "Probe completed successfully",
      }),
    );

    expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("subagent-result-text").textContent).toBe(
      "Probe completed successfully",
    );
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
