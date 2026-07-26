import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { TurnGroupMessage } from "./turn-group-message";

function toolExecute(
  id: string,
  output: Record<string, unknown> = {},
  command = "gh pr checks",
): Message {
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
          output: { exit_code: 0, ...output },
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

function staleCodexSubagent(): Message {
  return {
    id: "codex-subagent",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "ten_second_probe",
    type: "tool_call",
    turn_id: "turn-codex",
    created_at: "2026-07-23T12:00:00Z",
    metadata: {
      status: "in_progress",
      tool_call_id: "codex-subagent-tool",
      normalized: {
        kind: "subagent_task",
        subagent_task: {
          description: "ten_second_probe",
          subagent_type: "subagent",
          status: "started",
          child_session_id: "child-session",
        },
      },
    },
  };
}

function renderCodexSubagentGroup(isTurnActive: boolean): string {
  return renderToStaticMarkup(
    <TurnGroupMessage
      group={{
        type: "turn_group",
        id: "turn-group-codex",
        turnId: "turn-codex",
        messages: [staleCodexSubagent()],
      }}
      sessionId="s1"
      permissionsByToolCallId={new Map()}
      isLastGroup
      isTurnActive={isTurnActive}
    />,
  );
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

  it("does not summarize commands when only their output bodies distinguish them", () => {
    const messages = Array.from({ length: 6 }, (_, i) =>
      toolExecute(`tool-${i + 1}`, { has_output: true, stdout_bytes: 8, stderr_bytes: 0 }),
    );

    const html = renderToStaticMarkup(
      <TurnGroupMessage
        group={{
          type: "turn_group",
          id: "turn-group-output-tool-1",
          turnId: "turn-1",
          messages,
        }}
        sessionId="s1"
        permissionsByToolCallId={new Map()}
        isLastGroup
        isTurnActive
      />,
    );

    expect(html).not.toContain('data-testid="repeated-tool-summary"');
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

describe("TurnGroupMessage Codex subagent activity", () => {
  it("runs the group only while the stale lifecycle belongs to an active turn", () => {
    const activeHtml = renderCodexSubagentGroup(true);
    const settledHtml = renderCodexSubagentGroup(false);

    expect(activeHtml).toContain('aria-label="Loading"');
    expect(activeHtml).toContain("Subagent working...");
    expect(settledHtml).not.toContain('aria-label="Loading"');
    expect(settledHtml).not.toContain("Subagent working...");
  });
});
