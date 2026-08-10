/**
 * Parse and reassemble the optional workflow-level instructions block that the
 * orchestrator prepends at step entry.
 *
 * The heading and end marker are stable, agent-facing tokens (English, not i18n)
 * so chat can collapse the section by default without relying on message
 * metadata alone. The end marker is required: without it, ordinary user text
 * that happens to start with the heading is left alone.
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

function isLineBoundary(text: string, index: number): boolean {
  return index >= text.length || text[index] === "\n" || text[index] === "\r";
}

/** First standalone end marker (own line), not a substring mid-sentence. */
function findStandaloneEndMarker(text: string): number {
  let from = 0;
  while (from <= text.length) {
    const idx = text.indexOf(WORKFLOW_INSTRUCTIONS_END, from);
    if (idx === -1) return -1;
    const beforeOk = idx === 0 || text[idx - 1] === "\n" || text[idx - 1] === "\r";
    const afterOk = isLineBoundary(text, idx + WORKFLOW_INSTRUCTIONS_END.length);
    if (beforeOk && afterOk) return idx;
    from = idx + 1;
  }
  return -1;
}

/**
 * Split a user/auto-start message into workflow instructions + remainder.
 * Requires a leading exact heading line plus a standalone end marker.
 */
export function splitWorkflowInstructions(content: string): WorkflowInstructionsSplit {
  const text = content ?? "";
  const trimmedStart = text.replace(/^\uFEFF?/, "");
  if (!trimmedStart.startsWith(WORKFLOW_INSTRUCTIONS_HEADING)) {
    return { instructions: "", rest: text, hasInstructions: false };
  }
  // Exact heading line only — reject "## Workflow instructions for release".
  if (!isLineBoundary(trimmedStart, WORKFLOW_INSTRUCTIONS_HEADING.length)) {
    return { instructions: "", rest: text, hasInstructions: false };
  }

  // Orchestrator emits:
  //   "## Workflow instructions\n\n{body}\n\n<!-- /workflow-instructions -->\n\n{rest}"
  const afterHeading = trimmedStart.slice(WORKFLOW_INSTRUCTIONS_HEADING.length).replace(/^\n+/, "");
  const endIdx = findStandaloneEndMarker(afterHeading);
  if (endIdx === -1) {
    // No structural end marker: leave ordinary user content alone.
    return { instructions: "", rest: text, hasInstructions: false };
  }

  const instructions = afterHeading.slice(0, endIdx).replace(/\n+$/, "");
  const afterEnd = afterHeading
    .slice(endIdx + WORKFLOW_INSTRUCTIONS_END.length)
    .replace(/^\n+/, "");
  return {
    instructions,
    rest: afterEnd,
    hasInstructions: true,
  };
}
