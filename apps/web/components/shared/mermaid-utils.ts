/** Shared constants and helpers for mermaid diagram rendering. */

export const DEFAULT_SCALE = 0.75;
export const SCALE_STEP = 0.1;
export const MIN_SCALE = 0.1;
export const MAX_SCALE = 1.5;

/** Read intrinsic width/height from an SVG element's viewBox or attributes. */
export function getSvgDimensions(container: HTMLElement): { w: number; h: number } | null {
  const svg = container.querySelector("svg");
  if (!svg) return null;
  const vb = svg.getAttribute("viewBox");
  if (vb) {
    const parts = vb.split(/[\s,]+/).map(Number);
    if (parts.length === 4 && parts[2] > 0 && parts[3] > 0) {
      return { w: parts[2], h: parts[3] };
    }
  }
  const w = parseFloat(svg.getAttribute("width") ?? "");
  const h = parseFloat(svg.getAttribute("height") ?? "");
  if (w > 0 && h > 0) return { w, h };
  return null;
}
