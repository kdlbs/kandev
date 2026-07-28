export const PULL_TO_REFRESH_THRESHOLD = 72;

export function isAtVerticalScrollTop(
  target: EventTarget | null,
  root: HTMLElement | null,
): boolean {
  let current = target instanceof HTMLElement ? target : null;
  while (current) {
    if (current.scrollHeight > current.clientHeight && current.scrollTop > 0) return false;
    if (current === root) break;
    current = current.parentElement;
  }
  return true;
}

export function pullDistance(verticalDistance: number): number {
  return Math.min(Math.max(verticalDistance, 0) * 0.55, PULL_TO_REFRESH_THRESHOLD);
}

export function shouldRefreshAfterPull(distance: number): boolean {
  return distance >= PULL_TO_REFRESH_THRESHOLD;
}
