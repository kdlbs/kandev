export type ChangesTimelineNavigationKey = "ArrowDown" | "ArrowUp" | "Home" | "End";

export function changesTimelineNextIndex(
  key: ChangesTimelineNavigationKey,
  currentIndex: number,
  count: number,
): number {
  if (count <= 0) return -1;
  if (key === "Home") return 0;
  if (key === "End") return count - 1;
  return Math.min(Math.max(currentIndex + (key === "ArrowDown" ? 1 : -1), 0), count - 1);
}

export function isChangesTimelineTextEntry(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  return Boolean(
    target.closest(
      'input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="textbox"]',
    ),
  );
}
