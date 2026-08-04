/**
 * Parse and reassemble the optional workflow-level instructions block that the
 * orchestrator prepends at step entry.
 *
 * The heading is a stable, agent-facing marker (English, not i18n) so chat can
 * collapse it by default without relying on message metadata alone.
 */

export const WORKFLOW_INSTRUCTIONS_HEADING = "## Workflow instructions";

export type WorkflowInstructionsSplit = {
  /** Body under the heading, without the heading itself. Empty when absent. */
  instructions: string;
  /** Remainder of the message after the workflow block (step/task prompt). */
  rest: string;
  /** True when the message begins with the workflow instructions heading. */
  hasInstructions: boolean;
};

/**
 * Split a user/auto-start message into workflow instructions + remainder.
 * Only treats a leading `## Workflow instructions` section as the block.
 */
export function splitWorkflowInstructions(content: string): WorkflowInstructionsSplit {
  const text = content ?? "";
  const trimmedStart = text.replace(/^\uFEFF?/, "");
  if (!trimmedStart.startsWith(WORKFLOW_INSTRUCTIONS_HEADING)) {
    return { instructions: "", rest: text, hasInstructions: false };
  }

  // Orchestrator emits: "## Workflow instructions\n\n{body}\n\n{rest}"
  // Strip the blank line that immediately follows the heading.
  const afterHeading = trimmedStart.slice(WORKFLOW_INSTRUCTIONS_HEADING.length).replace(/^\n+/, "");
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
