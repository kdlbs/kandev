import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ContextCompactionMessage } from "./context-compaction-message";

afterEach(cleanup);

const ACTIVE_LABEL = "Compacting context";
const STATUS_TEST_ID = "context-compaction-status";

function compactionMessage(
  payload: Record<string, unknown>,
  status: "running" | "complete" | "error" | "cancelled" = "running",
): Message {
  return {
    id: "compaction-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "Context compaction",
    type: "tool_call",
    created_at: "2026-10-31T00:00:00Z",
    metadata: {
      status,
      normalized: { kind: "context_compaction", context_compaction: payload },
    },
  };
}

describe("ContextCompactionMessage", () => {
  it("shows an active in-progress row while its containing turn is active", () => {
    render(
      <ContextCompactionMessage
        comment={compactionMessage({ compaction_id: "c1", status: "in_progress" })}
        isContainingTurnActive
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe(ACTIVE_LABEL);
    expect(screen.getByTestId("context-compaction-active-indicator")).toBeTruthy();
  });

  it("shows completion metadata and expands the retained summary with a button", () => {
    render(
      <ContextCompactionMessage
        comment={compactionMessage(
          {
            compaction_id: "c1",
            status: "completed",
            summary: "## Kept context\n\nThe decisions remain available.",
            trigger: "manual",
            pre_tokens: 31_300,
            post_tokens: 2_400,
            duration_ms: 26_800,
          },
          "complete",
        )}
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe("Context compacted");
    expect(screen.getByTestId("context-compaction-token-counts").textContent).toBe(
      "31,300 tokens → 2,400 tokens",
    );
    expect(screen.getByTestId("context-compaction-trigger").textContent).toBe("manual");
    expect(screen.getByTestId("context-compaction-duration").textContent).toBe("26.8s");

    const summaryButton = screen.getByRole("button", { name: "Show compaction summary" });
    expect(summaryButton.tagName).toBe("BUTTON");
    expect(screen.queryByText("The decisions remain available.")).toBeNull();
    fireEvent.click(summaryButton);
    expect(screen.getByText("The decisions remain available.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Hide compaction details" })).toBeTruthy();
  });

  it("shows a failed compaction error as expandable plain text", () => {
    render(
      <ContextCompactionMessage
        comment={compactionMessage(
          {
            compaction_id: "c2",
            status: "failed",
            error: "provider could not summarize",
          },
          "error",
        )}
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe("Context compaction failed");
    fireEvent.click(screen.getByRole("button", { name: "Show compaction error" }));
    expect(screen.getByTestId("context-compaction-error").textContent).toBe(
      "provider could not summarize",
    );
  });

  it("does not show a stale in-progress compaction as active after its turn ends", () => {
    render(
      <ContextCompactionMessage
        comment={compactionMessage({ compaction_id: "c3", status: "in_progress" })}
        isContainingTurnActive={false}
      />,
    );

    expect(screen.queryByTestId("context-compaction-active-indicator")).toBeNull();
    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe("Compaction status is unconfirmed");
  });
});
