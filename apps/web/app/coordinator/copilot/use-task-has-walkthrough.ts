import { useAppStore } from "@/components/state-provider";

/**
 * Whether the task page shows the walkthrough launcher: it renders exactly when
 * the store holds a walkthrough for the task (components/review/walkthrough-overlay.tsx).
 */
export function useTaskHasWalkthrough(taskId: string | null): boolean {
  return useAppStore((s) => (taskId ? Boolean(s.walkthroughs.byTaskId[taskId]) : false));
}
