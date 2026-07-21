/** Base left padding (px) for a root sidebar row — matches the `pl-3` class. */
const BASE_PADDING_PX = 12;
/** Extra left padding (px) added per nesting level. Depth 1 = 32px ≈ `pl-8`. */
const INDENT_STEP_PX = 20;
/**
 * Maximum visual nesting depth. Deeper rows keep nesting logically but stop
 * indenting so a tall tree can't push titles off a narrow sidebar.
 */
const MAX_VISUAL_DEPTH = 6;

export type RowIndent = {
  /** Clamped depth used for layout. >0 means render the nesting connector. */
  depth: number;
  paddingLeftPx: number;
  /** Left offset (px) for the `↳` connector glyph. Only used when depth > 0. */
  connectorLeftPx: number;
};

/**
 * Resolve a row's tree depth from the explicit `depth` prop, falling back to
 * the legacy boolean `isSubTask` (depth 1) for callers that predate nesting.
 */
export function resolveRowDepth(depth: number | undefined, isSubTask: boolean | undefined): number {
  if (typeof depth === "number") return depth;
  return isSubTask ? 1 : 0;
}

/**
 * Resolve the indentation for a sidebar task row at a given tree depth.
 * Depth 1 reproduces the legacy single-level subtask look exactly; deeper
 * levels add a fixed step, capped at {@link MAX_VISUAL_DEPTH}.
 */
export function computeRowIndent(depth: number): RowIndent {
  const clamped = Math.min(Math.max(depth, 0), MAX_VISUAL_DEPTH);
  return {
    depth: clamped,
    paddingLeftPx: BASE_PADDING_PX + clamped * INDENT_STEP_PX,
    connectorLeftPx: BASE_PADDING_PX + 2 + (clamped - 1) * INDENT_STEP_PX,
  };
}
