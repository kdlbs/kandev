/** Shared constants and helpers for mermaid diagram rendering. */

export const DEFAULT_SCALE = 0.75;
export const SCALE_STEP = 0.1;
export const MIN_SCALE = 0.1;
export const MAX_SCALE = 1.5;

export const MERMAID_ERROR_EVENT = "mermaid:render-error";

export function cleanupMermaidOrphans(id: string): void {
  document.getElementById(id)?.remove();
  document.getElementById(`d${id}`)?.remove();
}

export function emitMermaidRenderError(message: string): void {
  document.dispatchEvent(new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message } }));
}

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

/**
 * Characters that require quoting in mermaid node/edge labels.
 * Includes: $, #, &, /, and other special chars that cause lexical errors.
 */
const SPECIAL_CHARS_RE = /[$#&/\\<>{}]/;

/**
 * Preprocesses mermaid code to quote text containing special characters.
 * Mermaid requires quotes around text with special chars like $, /, etc.
 *
 * Handles:
 * - Node labels: A[text] -> A["text"] if text contains special chars
 * - Edge labels: -->|text| -> -->|"text"| if text contains special chars
 * - Subgraph titles: subgraph text -> subgraph "text" if text contains special chars
 */
export function sanitizeMermaidCode(code: string): string {
  // Quote node labels: [text] or (text) or {text} or ([text]) or [[text]] etc.
  // Match brackets that aren't already quoted
  let result = code.replace(/(\[+)([^\]"]+?)(\]+)/g, (match, open, text, close) => {
    if (SPECIAL_CHARS_RE.test(text) && !text.startsWith('"')) {
      return `${open}"${text}"${close}`;
    }
    return match;
  });

  // Quote edge labels: |text|
  result = result.replace(/\|([^|"]+?)\|/g, (match, text) => {
    if (SPECIAL_CHARS_RE.test(text) && !text.startsWith('"')) {
      return `|"${text}"|`;
    }
    return match;
  });

  // Quote parentheses labels: (text) for stadium/circle nodes
  result = result.replace(/(\(+)([^)"]+?)(\)+)/g, (match, open, text, close) => {
    // Skip if it looks like a subgraph or keyword
    if (/^(subgraph|end|graph|flowchart|sequenceDiagram)\b/.test(text.trim())) {
      return match;
    }
    if (SPECIAL_CHARS_RE.test(text) && !text.startsWith('"')) {
      return `${open}"${text}"${close}`;
    }
    return match;
  });

  return result;
}
