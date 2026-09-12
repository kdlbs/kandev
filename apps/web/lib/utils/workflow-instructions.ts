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

// One-time move-entry instructions use the same sentinel shape but distinct
// tokens (kept in sync with apps/backend/internal/workflow/move/overlay.go).
// Unlike the durable block they are appended after the step/task prompt.
export const MOVE_INSTRUCTIONS_HEADING = "## One-time workflow move instructions";
export const MOVE_INSTRUCTIONS_END = "<!-- /one-time-workflow-move-instructions -->";

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

/** First standalone marker (its own line), not a substring mid-sentence. */
function findStandaloneMarker(text: string, marker: string, from: number): number {
  let cursor = from;
  while (cursor <= text.length) {
    const idx = text.indexOf(marker, cursor);
    if (idx === -1) return -1;
    const beforeOk = idx === 0 || text[idx - 1] === "\n" || text[idx - 1] === "\r";
    const afterOk = isLineBoundary(text, idx + marker.length);
    if (beforeOk && afterOk) return idx;
    cursor = idx + 1;
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
  const endIdx = findStandaloneMarker(afterHeading, WORKFLOW_INSTRUCTIONS_END, 0);
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

/** Which instruction block a segment carries: durable vs one-time move. */
export type InstructionSegmentKind = "workflow" | "move";

/** An ordered piece of a rendered message: plain text or a collapsible block. */
export type MessageSegment =
  | { type: "text"; content: string }
  | { type: "instructions"; kind: InstructionSegmentKind; content: string };

type MoveInstructionsSplit = {
  before: string;
  instructions: string;
  after: string;
  hasInstructions: boolean;
};

/**
 * Split the one-time move-instructions block from the surrounding message text.
 * The block may appear anywhere, but both its heading and end marker must be
 * standalone lines so ordinary user text is left alone.
 */
export function splitMoveInstructions(content: string): MoveInstructionsSplit {
  const text = content ?? "";
  const headingIdx = findStandaloneMarker(text, MOVE_INSTRUCTIONS_HEADING, 0);
  if (headingIdx === -1) {
    return { before: text, instructions: "", after: "", hasInstructions: false };
  }
  const bodyStart = headingIdx + MOVE_INSTRUCTIONS_HEADING.length;
  const endIdx = findStandaloneMarker(text, MOVE_INSTRUCTIONS_END, bodyStart);
  if (endIdx === -1) {
    return { before: text, instructions: "", after: "", hasInstructions: false };
  }
  const before = text.slice(0, headingIdx).replace(/\n+$/, "");
  const instructions = text.slice(bodyStart, endIdx).replace(/^\n+/, "").replace(/\n+$/, "");
  const after = text.slice(endIdx + MOVE_INSTRUCTIONS_END.length).replace(/^\n+/, "");
  return { before, instructions, after, hasInstructions: true };
}

function pushText(segments: MessageSegment[], content: string): void {
  if (content.trim() !== "") {
    segments.push({ type: "text", content });
  }
}

/**
 * Split a user/auto-start message into ordered render segments: the leading
 * durable workflow-instructions block, the trailing one-time move-instructions
 * block, and the plain text around them. Each instruction block is emitted so
 * chat can collapse it behind a labeled toggle instead of leaking raw markers.
 */
export function splitMessageSegments(content: string): MessageSegment[] {
  const text = content ?? "";
  const segments: MessageSegment[] = [];

  const durable = splitWorkflowInstructions(text);
  if (durable.hasInstructions) {
    segments.push({ type: "instructions", kind: "workflow", content: durable.instructions });
  }

  const remainder = durable.hasInstructions ? durable.rest : text;
  const move = splitMoveInstructions(remainder);
  if (!move.hasInstructions) {
    pushText(segments, remainder);
    return segments;
  }
  pushText(segments, move.before);
  segments.push({ type: "instructions", kind: "move", content: move.instructions });
  pushText(segments, move.after);
  return segments;
}
