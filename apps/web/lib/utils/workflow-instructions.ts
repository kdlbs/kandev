/**
 * Parse and reassemble the optional workflow-level instructions block that the
 * orchestrator prepends at step entry.
 *
 * The heading and end marker are stable, agent-facing tokens (English, not i18n)
 * so chat can collapse the section by default without relying on message
 * metadata alone. The end marker is required for multi-paragraph bodies — a
 * first-blank-line split would cut mid-instructions.
 */

export const WORKFLOW_INSTRUCTIONS_HEADING = "## Workflow instructions";
export const WORKFLOW_INSTRUCTIONS_END = "<!-- /workflow-instructions -->";

export type WorkflowInstructionsSplit = {
  /** Body under the heading, without the heading/end markers. Empty when absent. */
  instructions: string;
  /** Remainder of the message after the workflow block (step/task prompt). */
  rest: string;
  /** True when the message begins with the workflow instructions heading. */
  hasInstructions: boolean;
};

/**
 * Split a user/auto-start message into workflow instructions + remainder.
 * Only treats a leading `## Workflow instructions` section as the block.
 * Prefer the end marker when present; fall back to first blank line for older
 * messages that lack it.
 */
export function splitWorkflowInstructions(content: string): WorkflowInstructionsSplit {
  const text = content ?? "";
  const trimmedStart = text.replace(/^\uFEFF?/, "");
  if (!trimmedStart.startsWith(WORKFLOW_INSTRUCTIONS_HEADING)) {
    return { instructions: "", rest: text, hasInstructions: false };
  }

  // Orchestrator emits:
  //   "## Workflow instructions\n\n{body}\n\n<!-- /workflow-instructions -->\n\n{rest}"
  // Prefer the LAST end marker so a body that quotes the token cannot cut early.
  const afterHeading = trimmedStart.slice(WORKFLOW_INSTRUCTIONS_HEADING.length).replace(/^\n+/, "");

  const endIdx = afterHeading.lastIndexOf(WORKFLOW_INSTRUCTIONS_END);
  if (endIdx !== -1) {
    const instructions = afterHeading.slice(0, endIdx).replace(/\n+$/, "");
    const afterEnd = afterHeading.slice(endIdx + WORKFLOW_INSTRUCTIONS_END.length).replace(/^\n+/, "");
    return {
      instructions,
      rest: afterEnd,
      hasInstructions: true,
    };
  }

  // Legacy fallback: no end marker (messages recorded before the marker landed).
  const blankIdx = afterHeading.indexOf("\n\n");
  if (blankIdx === -1) {
    return {
      instructions: afterHeading.trimEnd(),
      rest: "",
      hasInstructions: true,
    };
  }
  return {
    instructions: afterHeading.slice(0, blankIdx).trimEnd(),
    rest: afterHeading.slice(blankIdx + 2),
    hasInstructions: true,
  };
}
