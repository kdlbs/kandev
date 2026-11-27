import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

// kandevToolCall constructs the kandev Message shape produced by the
// orchestrator for an MCP tool_call. The shape mirrors live production data
// observed on a real session:
//   - `metadata.tool_name`    — NOT set
//   - `metadata.title`        — raw tool name (`mcp__kandev__list_*_kandev`)
//   - `comment.content`       — same raw tool name
//   - `normalized.generic.name` — the ACP adapter's *category* ("other"),
//                                 NOT the tool name
// The matcher must therefore look at title/content, not generic.name —
// matching on the wrong field is exactly the bug that shipped initially.
export function kandevToolCall(opts: {
  toolName: string;
  input?: Record<string, unknown>;
  resultJson?: unknown;
  output?: unknown;
  status?: "pending" | "running" | "in_progress" | "complete" | "error";
}): Message {
  let output = opts.output;
  if (output === undefined && opts.resultJson !== undefined) {
    output = {
      _meta: null,
      content: [{ type: "text", text: JSON.stringify(opts.resultJson) }],
      structuredContent: null,
    };
  }
  return {
    id: "msg-1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: opts.toolName,
    type: "tool_call",
    created_at: "2026-05-21T10:00:00Z",
    metadata: {
      tool_call_id: "tc-1",
      title: opts.toolName,
      status: opts.status ?? "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: opts.input,
          output,
        },
      },
    },
  };
}
