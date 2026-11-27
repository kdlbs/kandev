import { PREVIEW_PANEL } from "./constants";

/**
 * The rendered preview panel width is the persisted chosen width floored to
 * the pointer-appropriate minimum. This never writes back to storage — the
 * chosen width itself is only ever floored to `MIN_WIDTH_PX` (see
 * `useKanbanPreview`).
 */
export function getRenderedPreviewPanelWidth(
  chosenWidthPx: number,
  isFinePointer: boolean,
): number {
  const minWidthPx = isFinePointer ? PREVIEW_PANEL.MIN_WIDTH_PX : PREVIEW_PANEL.COARSE_MIN_WIDTH_PX;
  return Math.max(chosenWidthPx, minWidthPx);
}
