import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { TurnGroupMessage } from "./turn-group-message";

function toolExecute(id: string, command = "gh pr checks"): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: command,
    type: "tool_execute",
    turn_id: "turn-1",
    created_at: "2026-05-30T10:00:00Z",
    metadata: {
      status: "complete",
      normalized: {
        shell_exec: {
          command,
          output: { exit_code: 0, has_output: true, stdout_bytes: 1, stderr_bytes: 0 },
        },
      },
    },
  };
}

function cancelledToolExecute(id: string): Message {
  const message = toolExecute(id);
  return {
    ...message,
    metadata: {
      ...message.metadata,
      status: "cancelled",
      normalized: {
        shell_exec: {
          command: "cancelled-command",
          output: { has_output: true, stdout_bytes: 16, stderr_bytes: 0 },
        },
      },
    },
  };
}

describe("TurnGroupMessage repeated tool compaction", () => {
  it("summarizes the middle of a long run of identical terminal commands", () => {
    const messages = Array.from({ length: 6 }, (_, i) => toolExecute(`tool-${i + 1}`));

    const html = renderToStaticMarkup(
      <TurnGroupMessage
        group={{
          type: "turn_group",
          id: "turn-group-tool-1",
          turnId: "turn-1",
          messages,
        }}
        sessionId="s1"
        permissionsByToolCallId={new Map()}
        isLastGroup
        isTurnActive
      />,
    );

    expect(html).toContain('data-testid="repeated-tool-summary"');
    expect(html).toContain("4 repeated identical terminal commands hidden");
  });

  it("treats a cancelled tool as terminal", () => {
    const html = renderToStaticMarkup(
      <TurnGroupMessage
        group={{
          type: "turn_group",
          id: "turn-group-cancelled",
          turnId: "turn-1",
          messages: [cancelledToolExecute("tool-cancelled")],
        }}
        sessionId="s1"
        permissionsByToolCallId={new Map()}
      />,
    );

    expect(html).not.toContain('aria-label="Loading"');
  });
});
