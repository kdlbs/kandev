import { useEffect, useLayoutEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";

// Any real counter the store can hold is >= 1, so 0 reliably "looks bumped"
// on the very first observed value and "looks unbumped" otherwise — see
// use-session-commits.ts for the same sentinel pattern.
const REFETCH_TRIGGER_INIT = 0;

/**
 * Calls `onRefetch` when a matching office refetch trigger fires.
 * Supports exact match ("dashboard") or prefix match ("comments:" matches "comments:task-123").
 *
 * Triggers are per-type counters (`office.refetchTriggers`), not a single
 * "last trigger" value: a WS handler often bumps several distinct types in
 * one synchronous call (e.g. `task:${id}` then `dashboard`), and React
 * coalesces those into one render, so a shared last-value field would only
 * ever be observable by the final type's subscribers.
 *
 * @param triggerType - The trigger type to watch for (e.g. "dashboard", "tasks", "comments")
 * @param onRefetch - Callback invoked when a matching trigger fires
 */
export function useOfficeRefetch(triggerType: string, onRefetch: () => void) {
  const triggers = useAppStore((s) => s.office.refetchTriggers);
  const callbackRef = useRef(onRefetch);
  // Update ref in a layout effect to avoid mutating during render
  useLayoutEffect(() => {
    callbackRef.current = onRefetch;
  });

  // Tracks the highest counter we've already acted on, per matching key, so
  // a bump triggers exactly one refetch rather than re-firing on every
  // render. A single scalar here would let a bump on one sub-key (e.g.
  // "comments:task-B") mask a later bump on a sibling sub-key ("comments:
  // task-A") whenever the sibling's counter happened to be lower.
  const lastSeenRef = useRef<Record<string, number>>({});

  useEffect(() => {
    let fire = false;
    for (const [type, seq] of Object.entries(triggers)) {
      if (type !== triggerType && !type.startsWith(triggerType + ":")) continue;
      if (seq > (lastSeenRef.current[type] ?? REFETCH_TRIGGER_INIT)) {
        lastSeenRef.current[type] = seq;
        fire = true;
      }
    }
    if (fire) callbackRef.current();
  }, [triggers, triggerType]);
}
