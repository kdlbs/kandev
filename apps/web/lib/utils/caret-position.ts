const MIRROR_STYLES = [
  "fontFamily",
  "fontSize",
  "fontWeight",
  "fontStyle",
  "letterSpacing",
  "textTransform",
  "wordSpacing",
  "textIndent",
  "whiteSpace",
  "wordWrap",
  "wordBreak",
  "overflowWrap",
  "lineHeight",
  "padding",
  "paddingTop",
  "paddingRight",
  "paddingBottom",
  "paddingLeft",
  "borderWidth",
  "boxSizing",
] as const;

export function measureCaretRect(textarea: HTMLTextAreaElement, value: string): DOMRect {
  const selectionStart = textarea.selectionStart;
  const computed = window.getComputedStyle(textarea);
  const mirror = document.createElement("div");

  mirror.style.position = "absolute";
  mirror.style.visibility = "hidden";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.wordWrap = "break-word";
  mirror.style.width = `${textarea.clientWidth}px`;
  MIRROR_STYLES.forEach((prop) => {
    mirror.style[prop as unknown as number] = computed[prop];
  });

  document.body.appendChild(mirror);

  mirror.textContent = value.substring(0, selectionStart);
  const marker = document.createElement("span");
  marker.textContent = "​";
  mirror.appendChild(marker);

  let caretLeft: number;
  let caretTop: number;
  try {
    const textareaRect = textarea.getBoundingClientRect();
    const markerRect = marker.getBoundingClientRect();
    const mirrorRect = mirror.getBoundingClientRect();
    const scrollTop = textarea.scrollTop;
    caretLeft = textareaRect.left + (markerRect.left - mirrorRect.left);
    caretTop = textareaRect.top + (markerRect.top - mirrorRect.top) - scrollTop;
  } finally {
    document.body.removeChild(mirror);
  }

  return new DOMRect(
    caretLeft,
    caretTop,
    0,
    parseInt(computed.lineHeight, 10) || parseInt(computed.fontSize, 10) * 1.2,
  );
}
