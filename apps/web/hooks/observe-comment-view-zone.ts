import type { editor as monacoEditor } from "monaco-editor";

export function observeCommentViewZone(
  editor: monacoEditor.ICodeEditor,
  zone: monacoEditor.IViewZone,
  zoneId: string,
  content: HTMLElement,
) {
  if (typeof ResizeObserver === "undefined") return () => {};
  const minimumHeight = zone.heightInPx ?? 0;
  const observer = new ResizeObserver(([entry]) => {
    if (!entry) return;
    const height = Math.max(minimumHeight, Math.ceil(entry.contentRect.height));
    if (height === zone.heightInPx) return;
    zone.heightInPx = height;
    editor.changeViewZones((accessor) => accessor.layoutZone(zoneId));
  });
  observer.observe(content);
  return () => observer.disconnect();
}
