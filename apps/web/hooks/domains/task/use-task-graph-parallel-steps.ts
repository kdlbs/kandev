import { useCallback, useState } from "react";

export const PARALLEL_STEPS_STORAGE_KEY = "kandev.taskGraph.parallelSteps.v1";

function readStored(): boolean {
  try {
    return window.localStorage.getItem(PARALLEL_STEPS_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

/** Device-local "Parallel steps" toggle shared by every graph surface. */
export function useTaskGraphParallelSteps(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(readStored);
  const update = useCallback((next: boolean) => {
    setOn(next);
    try {
      window.localStorage.setItem(PARALLEL_STEPS_STORAGE_KEY, String(next));
    } catch {
      // Storage can be unavailable (private mode); the toggle still works for this view.
    }
  }, []);
  return [on, update];
}
