import { useEffect, useLayoutEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";

/**
 * Calls `onRefetch` when a matching office refetch trigger fires.
 * Supports exact match ("dashboard") or prefix match ("comments:" matches "comments:task-123").
 *
 * Triggers are batched into a single `office.refetchTrigger` object per tick
 * rather than tracked as separate values: a WS handler often bumps several
 * distinct types in one synchronous call (e.g. `task:${id}` then `dashboard`),
 * and React coalesces those into one render, so per-type values would only
 * ever be observable by the final type's subscribers.
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
    const matches = trigger.types.some(
      (type) => type === triggerType || type.startsWith(triggerType + ":"),
    );
    if (matches) {
      callbackRef.current();
    }
  }, [trigger, triggerType]);
}
