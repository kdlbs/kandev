"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import { t } from "@/lib/i18n";
import { triggerAutomation } from "@/lib/api/domains/automation-api";

/**
 * How long to keep looking for the run a successful fire promised.
 *
 * `automation.trigger` publishes an event and returns; the orchestrator creates
 * the task and writes the `automation_runs` row on its own goroutine (see
 * `handleAutomationTriggered`). A single refresh on the response therefore
 * races that write, and losing the race is silent in the worst way: with no
 * open run the page also stops polling, so the user is left looking at
 * "Triggered" over an empty list until they reload by hand.
 *
 * Re-asking a few times over a few seconds costs one or two extra reads and
 * removes the race. It stops on its own rather than polling on: if the row has
 * not appeared by the end, something is wrong that another read will not fix.
 */
const RUN_APPEARANCE_RETRY_MS = [0, 400, 1200, 3000];

/**
 * How long a page stays live after a successful fire, regardless of what it can
 * already see.
 *
 * The retry burst above only covers the first few seconds. A caller that gates
 * its polling on "something is open" — which is every caller, because an idle
 * workspace should issue no requests — therefore has a hole: if the run row is
 * written after the burst, nothing is open when the burst ends, polling never
 * starts, and the page keeps rendering its pre-trigger snapshot until someone
 * presses Refresh. Anything derived from that snapshot goes stale with it,
 * including the amber note explaining why the automation will not fire next.
 *
 * A minute is long enough for a slow orchestrator to write the row and short
 * enough that a mis-fire does not leave a page polling forever.
 */
export const TRIGGER_SETTLE_WINDOW_MS = 60_000;

/**
 * Firing an automation by hand from its own page.
 *
 * A trigger can succeed and still run nothing — the concurrency cap turns the
 * request away while an earlier run is open — so the skip is reported as a skip
 * rather than as a fire. Reporting it as success is the failure mode that made
 * the settings-page trigger feel broken: the user clicked, saw "Triggered", and
 * no run appeared.
 */
export function useManualTrigger(automationId: string, onFired: () => void) {
  const [triggering, setTriggering] = useState(false);
  // True while a fired run is still expected to show up. Callers OR this into
  // whatever gates their live refresh, so the transition from "nothing open" to
  // "a run is open" and back is observed rather than inferred from a snapshot
  // taken before the run existed.
  const [settling, setSettling] = useState(false);
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);

  // Navigating away mid-retry must not leave a refresh pointed at an unmounted
  // page.
  useEffect(
    () => () => {
      for (const timer of timers.current) clearTimeout(timer);
      timers.current = [];
    },
    [],
  );

  const runNow = useCallback(async () => {
    if (!automationId || triggering) return;
    setTriggering(true);
    try {
      const result = await triggerAutomation(automationId);
      if (result?.skipped) {
        toast.info(
          result.reason
            ? t("automations:skipped", { reason: result.reason })
            : t("automations:skippedAlreadyRunning"),
        );
        return;
      }
      toast.success(t("automations:triggered"));
      // A second fire restarts the window rather than inheriting the first
      // one's expiry, which would otherwise stop the page mid-transition.
      for (const timer of timers.current) clearTimeout(timer);
      timers.current = [];
      setSettling(true);
      for (const delay of RUN_APPEARANCE_RETRY_MS) {
        timers.current.push(setTimeout(onFired, delay));
      }
      timers.current.push(setTimeout(() => setSettling(false), TRIGGER_SETTLE_WINDOW_MS));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("automations:couldNotTrigger"));
    } finally {
      setTriggering(false);
    }
  }, [automationId, onFired, triggering]);

  return { runNow, triggering, settling };
}
