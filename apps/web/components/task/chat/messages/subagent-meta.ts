import type { SubagentTaskPayload } from "@/components/task/chat/types";
import { t } from "@/lib/i18n";
import { formatNumber } from "@/lib/i18n/formats";

export type SubagentMetaChip = {
  /**
   * Stable identity for React keys and `data-testid`. Never translated — the
   * display label moved to `label` precisely so this can stay in English.
   */
  id: string;
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

// The count drives the plural rule; the pre-formatted string is what renders,
// so digit grouping follows the active locale instead of a hardcoded en-US.
function formatTokens(tokens: number): string {
  return t("task:tokenCount", { count: tokens, formatted: formatNumber(tokens) });
}

function formatTools(count: number): string {
  return t("task:toolCount", { count });
}

/**
 * Turns a SubagentTaskPayload into an ordered list of display chips. Different
 * agents populate different subsets (Claude: tokens/duration/tool_use_count;
 * OpenCode: model/child_session_id; Cursor: duration_ms only),
 * so each field is only included when meaningfully present. Duration and tokens
 * are skipped when zero; tool_use_count is shown even at zero since "0 tools"
 * is meaningful for a completed subagent.
 */
export function subagentMetaChips(
  payload: SubagentTaskPayload | undefined,
  childCount?: number,
): SubagentMetaChip[] {
  if (!payload) return [];

  const chips: SubagentMetaChip[] = [];

  // Claude Code's Task with run_in_background=true: the dispatch is terminal,
  // but the subagent runs out-of-band and writes its result to output_file.
  // Surface a "background" chip so the UI doesn't look like a normal completion.
  if (payload.is_async || payload.status === "async_launched") {
    chips.push({ id: "background", label: t("task:background"), value: t("task:background") });
  }
  if (typeof payload.duration_ms === "number" && payload.duration_ms > 0) {
    chips.push({
      id: "duration",
      label: t("task:duration"),
      value: formatDuration(payload.duration_ms),
    });
  }
  if (typeof payload.total_tokens === "number" && payload.total_tokens > 0) {
    chips.push({
      id: "tokens",
      label: t("task:tokens"),
      value: formatTokens(payload.total_tokens),
    });
  }
  // The header already states the child count ("10 tool calls"), so a chip
  // repeating the same number is noise. A divergence is not: it means the agent
  // reported more tool uses than it streamed, which is worth seeing. The header
  // renders no count at all when nothing streamed, so a zero child count can
  // never be the duplicate — and "0 tools" is exactly the case the chip is for.
  const headerStatesCount = typeof childCount === "number" && childCount > 0;
  if (
    typeof payload.tool_use_count === "number" &&
    !(headerStatesCount && payload.tool_use_count === childCount)
  ) {
    chips.push({ id: "tools", label: t("task:tools"), value: formatTools(payload.tool_use_count) });
  }
  if (payload.model) {
    chips.push({ id: "model", label: t("task:model"), value: payload.model });
  }
  if (payload.child_session_id) {
    chips.push({
      id: "session",
      label: t("task:session"),
      value: truncateId(payload.child_session_id),
    });
  }

  return chips;
}
