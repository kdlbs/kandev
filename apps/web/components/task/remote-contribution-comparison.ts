import { useSyncExternalStore } from "react";

export type ContributionComparisonRequest = {
  key: string;
  token: number;
};

let nextToken = 0;
let currentRequest: ContributionComparisonRequest | null = null;
const listeners = new Set<() => void>();

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return currentRequest;
}

export function requestContributionComparison(key: string) {
  currentRequest = { key, token: ++nextToken };
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent("switch-to-changes-tab"));
  }
  notify();
}

export function consumeContributionComparisonRequest(token: number) {
  if (currentRequest?.token !== token) return;
  currentRequest = null;
  notify();
}

export function useContributionComparisonRequest() {
  return useSyncExternalStore(subscribe, getSnapshot, () => null);
}
