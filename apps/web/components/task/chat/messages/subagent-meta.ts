import type { SubagentTaskPayload } from "@/components/task/chat/types";

export type SubagentMetaChip = {
  label: string;
  value: string;
};

const MAX_ID_LENGTH = 12;

function truncateId(value: string): string {
  if (value.length <= MAX_ID_LENGTH) return value;
  return `${value.slice(0, MAX_ID_LENGTH)}…`;
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatTokens(tokens: number): string {
  return `${tokens.toLocaleString("en-US")} tokens`;
}

function formatTools(count: number): string {
  return `${count} tool${count === 1 ? "" : "s"}`;
}

/**
 * Turns a SubagentTaskPayload into an ordered list of display chips. Different
 * agents populate different subsets (Claude: tokens/duration/tool_use_count;
 * OpenCode: model/child_session_id; Cursor: duration_ms only),
 * so each field is only included when meaningfully present. Duration and tokens
 * are skipped when zero; tool_use_count is shown even at zero since "0 tools"
 * is meaningful for a completed subagent.
 */
export function subagentMetaChips(payload: SubagentTaskPayload | undefined): SubagentMetaChip[] {
  if (!payload) return [];

  const chips: SubagentMetaChip[] = [];

  // Claude Code's Task with run_in_background=true: the dispatch is terminal,
  // but the subagent runs out-of-band and writes its result to output_file.
  // Surface a "background" chip so the UI doesn't look like a normal completion.
  if (payload.is_async || payload.status === "async_launched") {
    chips.push({ label: "background", value: "background" });
  }
  if (typeof payload.duration_ms === "number" && payload.duration_ms > 0) {
    chips.push({ label: "duration", value: formatDuration(payload.duration_ms) });
  }
  if (typeof payload.total_tokens === "number" && payload.total_tokens > 0) {
    chips.push({ label: "tokens", value: formatTokens(payload.total_tokens) });
  }
  if (typeof payload.tool_use_count === "number") {
    chips.push({ label: "tools", value: formatTools(payload.tool_use_count) });
  }
  if (payload.model) {
    chips.push({ label: "model", value: payload.model });
  }
  if (payload.child_session_id) {
    chips.push({ label: "session", value: truncateId(payload.child_session_id) });
  }

  return chips;
}
