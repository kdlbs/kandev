import type { AutomationRun } from "@/lib/types/automation";
import { STATE_DOT_CLASS, STATE_LABEL_KEY, type AutomationActivityState } from "./automation-rows";
import { isOpenRun, statusDotClass, statusLabelKey } from "./run-status";

/** Activity controls priority; newest-run metadata always describes the same run. */
export function automationRunPresentation(
  state: AutomationActivityState,
  lastRun: AutomationRun | null | undefined,
) {
  const outcomeLabelKey = lastRun ? statusLabelKey(lastRun.status) : null;
  const showOutcome = Boolean(
    lastRun && lastRun.status !== "succeeded" && !isOpenRun(lastRun.status),
  );
  const labelKeys: string[] = [];
  if (state !== "idle" || !lastRun) labelKeys.push(STATE_LABEL_KEY[state]);
  if (outcomeLabelKey && !(state === "running" && lastRun && isOpenRun(lastRun.status))) {
    labelKeys.push(outcomeLabelKey);
  }
  return {
    dotClass: state === "idle" && lastRun ? statusDotClass(lastRun.status) : STATE_DOT_CLASS[state],
    outcomeLabelKey,
    showOutcome,
    labelKeys,
  };
}
