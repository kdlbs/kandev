import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { isSubagentEffectivelyActive, ToolSubagentMessage } from "./tool-subagent-message";

afterEach(cleanup);

const CHILD_TOOL_LABEL = "Read SyncRunner.ts";
const COPILOT_PROMPT = "Read a.txt and report its contents.";
const GENERIC_PROMPT = "Inspect the lifecycle events";

function subagentMessage({
  status = "complete",
  payloadStatus = "completed",
  provider,
  name,
  description,
  subagentType = "subagent",
  prompt,
  resultText,
  resumeCount,
}: {
  status?: string;
  payloadStatus?: string;
  provider?: "copilot";
  name?: string;
  description?: string;
  subagentType?: string;
  prompt?: string;
  resultText?: string;
  resumeCount?: number;
} = {}): Message {
  return {
    id: "subagent",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "tool_call",
    content: "Subagent",
    created_at: "2026-07-23T12:00:00Z",
    metadata: {
      status,
      normalized: {
        kind: "subagent_task",
        subagent_task: {
          provider,
          name,
          description,
          subagent_type: subagentType,
          status: payloadStatus,
          prompt,
          result_text: resultText,
          resume_count: resumeCount,
        },
      },
    },
  };
}

function childTool(): Message {
  return {
    id: "child-tool",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "tool_call",
    content: CHILD_TOOL_LABEL,
    created_at: "2026-07-23T12:00:01Z",
    metadata: { status: "complete", tool_call_id: "child-tool" },
  };
}

function renderSubagent(
  message: Message,
  isContainingTurnActive = false,
  children: Message[] = [],
) {
  return render(
    <ToolSubagentMessage
      comment={message}
      childMessages={children}
      isContainingTurnActive={isContainingTurnActive}
      renderChild={(child) => <span>{child.content}</span>}
    />,
  );
}

describe("subagent provider presentation", () => {
  it("uses the containing turn to decide whether a running payload is active", () => {
    const message = subagentMessage({ payloadStatus: "running" });
    const metadata = message.metadata as ToolCallMetadata;
    expect(isSubagentEffectivelyActive(metadata, false)).toBe(false);
    expect(isSubagentEffectivelyActive(metadata, true)).toBe(true);

    renderSubagent(message);
    expect(screen.queryByText("Working...")).toBeNull();
  });

  it("shows a resumed count while the subagent is active", () => {
    renderSubagent(
      subagentMessage({ status: "running", payloadStatus: "running", resumeCount: 1 }),
      true,
    );
    expect(screen.getByTestId("subagent-meta-resume").textContent).toBe("Resumed x1");
  });

  it("keeps other providers' prompt as a fallback only", () => {
    renderSubagent(
      subagentMessage({
        prompt: GENERIC_PROMPT,
        resultText: "The subagent completed the inspection.",
      }),
      false,
      [childTool()],
    );

    fireEvent.click(screen.getByRole("button"));
    expect(screen.queryByText(GENERIC_PROMPT)).toBeNull();
    expect(screen.getByTestId("subagent-result-text").textContent).toBe(
      "The subagent completed the inspection.",
    );
    expect(screen.getByText(CHILD_TOOL_LABEL)).toBeTruthy();
  });

  it("shows Copilot identity and prompt after child calls", () => {
    renderSubagent(
      subagentMessage({
        provider: "copilot",
        name: "reader",
        description: "Read a.txt contents",
        subagentType: "explore",
        prompt: COPILOT_PROMPT,
        resultText: "The file contains apple.",
      }),
      false,
      [childTool()],
    );

    expect(screen.getByTestId("subagent-name").textContent).toBe("reader");
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getAllByText("Read a.txt contents")).toHaveLength(1);
    expect(screen.getByTestId("subagent-prompt").textContent).toBe(COPILOT_PROMPT);
    expect(screen.getByTestId("subagent-result-text").textContent).toBe("The file contains apple.");
    const cardText = screen.getByTestId("subagent-card").textContent ?? "";
    expect(cardText.indexOf(CHILD_TOOL_LABEL)).toBeLessThan(cardText.indexOf(COPILOT_PROMPT));
  });
});
