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
 * `docs/specs/coordinator/system-design/copilot-panel.md#ask-about-this`:
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

export type CopilotItemRefKind = "task" | "proposal" | "stall" | "workflow";

export type CopilotItemRef = { kind: CopilotItemRefKind; id: string };

/**
 * The **Ask about this** wire reference for a card
 * (`docs/specs/coordinator/system-design/copilot-panel.md#ask-about-this`):
 * a proposal item references its own proposal id, kind `"proposal"`; every
 * other kind (question, stall, error, and every Queue row) references its
 * task id. This is independent of `deriveCopilotItemId`'s display `<id>`,
 * which for a proposal with a source task is that task's identifier, not the
 * proposal id.
 */
export function deriveCopilotItemRef(item: NeedsYouItem | QueueItem): CopilotItemRef {
  if ("group" in item) return { kind: "task", id: item.task.id };
  if (item.kind === "proposal") return { kind: "proposal", id: item.proposal.id };
  if (item.kind === "stall") return { kind: "stall", id: item.task.id };
  return { kind: "task", id: item.task.id };
}
