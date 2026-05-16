import { useEffect, useLayoutEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";

/**
 * Calls `onRefetch` when the office refetch trigger matches `triggerType`.
 * Supports exact match ("dashboard") or prefix match ("comments:" matches "comments:task-123").
 *
 * @param triggerType - The trigger type to watch for (e.g. "dashboard", "tasks", "comments")
 * @param onRefetch - Callback invoked when a matching trigger fires
 */
export function useOfficeRefetch(triggerType: string, onRefetch: () => void) {
  const trigger = useAppStore((s) => s.office.refetchTrigger);
  const callbackRef = useRef(onRefetch);
  // Update ref in a layout effect to avoid mutating during render
  useLayoutEffect(() => {
    callbackRef.current = onRefetch;
  });

  useEffect(() => {
    if (!trigger) return;
    const matches = trigger.type === triggerType || trigger.type.startsWith(triggerType + ":");
    if (matches) {
      callbackRef.current();
    }
  }, [trigger, triggerType]);
}
