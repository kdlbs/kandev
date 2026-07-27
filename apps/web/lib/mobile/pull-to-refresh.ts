export const PULL_TO_REFRESH_THRESHOLD = 72;

export function pullDistance(verticalDistance: number): number {
  return Math.min(Math.max(verticalDistance, 0) * 0.55, PULL_TO_REFRESH_THRESHOLD);
}

export function shouldRefreshAfterPull(distance: number): boolean {
  return distance >= PULL_TO_REFRESH_THRESHOLD;
}
