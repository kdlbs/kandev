import type { AttentionTask, NeedsYouItem, QueueItem } from "./attention";
import { resolveProposalSourceTask } from "./item-text";

/**
 * Runs of whitespace (line breaks included) collapse to one space, the
 * result is trimmed, and each leftover `": "` is replaced with `" - "` until
 * none remains, so the transcript parser
 * (`AC-COORDINATOR-COPILOT-005.3`) can always read the id back whole from
 * the `About <id>: ` prefix.
 */
export function normalizeCopilotItemId(raw: string): string {
  const collapsed = raw.replace(/\s+/g, " ").trim();
  let result = collapsed;
  while (result.includes(": ")) result = result.replace(": ", " - ");
  return result;
}

function rawCopilotItemId(
  item: NeedsYouItem | QueueItem,
  openTasksById: Map<string, AttentionTask>,
): string {
  if ("group" in item) return item.task.identifier ?? item.task.title;
  if (item.kind === "proposal") {
    const sourceTask = resolveProposalSourceTask(item, openTasksById);
    if (sourceTask) return sourceTask.identifier ?? sourceTask.title;
    return item.proposal.spec.title;
  }
  return item.task.identifier ?? item.task.title;
}

/**
 * The **Ask about this** `<id>` for a card, per
 * `docs/specs/coordinator/system-design/copilot-popover.md#ask-about-this`:
 * a question, stall or error item uses its task's identifier, else title; a
 * proposal with a source task uses that task's identifier, else its title; a
 * proposal without one uses the proposal's own title (never a "New task"
 * fallback, unlike the card head).
 */
export function deriveCopilotItemId(
  item: NeedsYouItem | QueueItem,
  openTasksById: Map<string, AttentionTask>,
): string {
  return normalizeCopilotItemId(rawCopilotItemId(item, openTasksById));
}
