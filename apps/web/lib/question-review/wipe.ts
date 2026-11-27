/** Pixels at each stage edge where the divider is already at 0% or 100%. */
export const WIPE_DRAG_MARGIN = 24;

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

/**
 * Divider position for a pointer. The track is the stage minus a margin on each
 * side, so a finger well inside a phone screen still reaches 0% and 100%.
 */
export function wipeFromPointer(clientX: number, stage: { left: number; width: number }): number {
  if (stage.width <= 0) return 0.5;
  const margin = stage.width > WIPE_DRAG_MARGIN * 4 ? WIPE_DRAG_MARGIN : 0;
  return clamp((clientX - stage.left - margin) / (stage.width - margin * 2), 0, 1);
}

/**
 * Divider position after a drag: the split at the start of the drag moved by
 * the horizontal travel only, on the same track as `wipeFromPointer`. Grabbing
 * the grip beside the line, or moving it straight up or down, keeps the split.
 */
export function wipeFromDrag(
  start: number,
  dx: number,
  stage: { left: number; width: number },
): number {
  if (stage.width <= 0) return start;
  const margin = stage.width > WIPE_DRAG_MARGIN * 4 ? WIPE_DRAG_MARGIN : 0;
  return clamp(start + dx / (stage.width - margin * 2), 0, 1);
}
