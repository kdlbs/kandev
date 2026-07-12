import { useCallback, useSyncExternalStore } from "react";

type Listener = () => void;

let openTaskId: string | null = null;
const listeners = new Set<Listener>();

function emit() {
  for (const listener of listeners) listener();
}

export function setOpenWalkthroughTaskId(taskId: string | null): void {
  if (openTaskId === taskId) return;
  openTaskId = taskId;
  emit();
}

export function clearOpenWalkthroughTaskId(taskId: string): void {
  if (openTaskId !== taskId) return;
  openTaskId = null;
  emit();
}

export function getOpenWalkthroughTaskId(): string | null {
  return openTaskId;
}

export function subscribeOpenWalkthroughTask(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useIsWalkthroughOpenForTask(taskId: string | null | undefined): boolean {
  const getSnapshot = useCallback(() => !!taskId && openTaskId === taskId, [taskId]);
  return useSyncExternalStore(subscribeOpenWalkthroughTask, getSnapshot, getSnapshot);
}
