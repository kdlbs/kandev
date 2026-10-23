import type { MarkdownInputFactoryOptions } from "./markdown-input-transport";

/** Only rendered text is decorative; inline editor actions remain accessible. */
export function hideMarkdownDecoration(host: HTMLElement): void {
  for (const element of host.querySelectorAll(
    ".md-text, .md-marker, .md-selection-layer, .md-cursor",
  )) {
    const interactive = "a, button, input, textarea, select, [tabindex], [role=button]";
    const ancestor = element.closest(interactive);
    if ((!ancestor || ancestor === host) && !element.querySelector(interactive))
      element.setAttribute("aria-hidden", "true");
  }
}

/** Engine overlay rectangles are local CSS pixels; browser input bounds are client pixels. */
export function getMarkdownClientCaretRect(
  options: MarkdownInputFactoryOptions,
): DOMRect | undefined {
  const caret = options.getCaretRect();
  if (!caret) return;
  const host = options.host;
  const overlay = host.querySelector<SVGSVGElement>(".md-selection-layer");
  const matrix = overlay?.getScreenCTM?.();
  const container = host.querySelector<HTMLElement>(".md-editor-content") ?? host;
  const origin = container.getBoundingClientRect();
  const left = matrix
    ? matrix.a * caret.left + matrix.e
    : origin.left + caret.left - container.scrollLeft;
  const top = matrix
    ? matrix.d * caret.top + matrix.f
    : origin.top + caret.top - container.scrollTop;
  return new DOMRect(left, top, caret.width * (matrix?.a ?? 1), caret.height * (matrix?.d ?? 1));
}

export function positionMarkdownInput(
  options: MarkdownInputFactoryOptions,
  input: HTMLTextAreaElement,
): void {
  const caret = getMarkdownClientCaretRect(options);
  if (!caret) return;
  const host = options.host;
  const hostRect = host.getBoundingClientRect();
  const scaleX = host.offsetWidth ? hostRect.width / host.offsetWidth : 1;
  const scaleY = host.offsetHeight ? hostRect.height / host.offsetHeight : 1;
  input.style.left = `${(caret.left - hostRect.left) / (scaleX || 1) + host.scrollLeft}px`;
  input.style.top = `${(caret.top - hostRect.top) / (scaleY || 1) + host.scrollTop}px`;
  input.style.height = `${Math.max(1, caret.height / (scaleY || 1))}px`;
}
