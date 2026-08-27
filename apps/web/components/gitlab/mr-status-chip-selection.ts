"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { compareChipMR } from "./mr-task-icon";
import {
  autoFixRoundForState,
  findMRAutomationOptionsForMR,
  findMRAutomationStateForMR,
  type AutoFixRoundInfo,
} from "@/lib/gitlab/mr-automation";
import type { TaskMR, TaskMRAutomationOptions } from "@/lib/types/gitlab";

export type ChipAutomation = {
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
  autoFixRound: AutoFixRoundInfo | null;
};

function autoFixRoundFor(options: TaskMRAutomationOptions | null, mr: TaskMR): AutoFixRoundInfo {
  return autoFixRoundForState(
    findMRAutomationStateForMR(options?.mr_states, mr),
    options?.auto_fix_max_rounds,
  );
}

/**
 * One MR's own automation switches. The response's top-level booleans are an
 * aggregate ("every linked MR has this on, and at least one MR is linked"),
 * so reading them per MR would hide a switch the user enabled on only some
 * of the linked MRs — the chip would show no auto-fix badge and no round
 * counter while auto-fix was actively spending its round budget on one MR.
 * Falls back to the aggregate only when `mr_options` is absent, i.e. a
 * payload that predates per-MR scoping. Mirrors GitHub's automationForPR.
 */
function switchesForMR(
  options: TaskMRAutomationOptions | null,
  mr: TaskMR,
): { autoFix: boolean; autoMerge: boolean } {
  if (!options?.mr_options) {
    return {
      autoFix: Boolean(options?.auto_fix_enabled),
      autoMerge: Boolean(options?.auto_merge_enabled),
    };
  }
  const row = findMRAutomationOptionsForMR(options.mr_options, mr);
  return { autoFix: row.auto_fix_enabled, autoMerge: row.auto_merge_enabled };
}

function isBadgeMRBetter(
  candidateRound: AutoFixRoundInfo,
  candidate: TaskMR,
  bestRound: AutoFixRoundInfo,
  best: TaskMR,
): boolean {
  if (candidateRound.exhausted !== bestRound.exhausted) return candidateRound.exhausted;
  if (candidateRound.current !== bestRound.current)
    return candidateRound.current > bestRound.current;
  return compareChipMR(candidate, best) < 0;
}

/**
 * The badge-selected MR: the most attention-worthy auto-fix round across the
 * open MRs that have auto-fix enabled *for themselves* — an exhausted round
 * beats a non-exhausted one, then a higher `current` wins, then the same
 * `mr_iid` / `project_path` / `id` order `selectChipMR` uses (spec: Selection
 * and ordering). Distinct from the live and frozen selections, and
 * deliberately never frozen (spec: "The automation badges do NOT freeze").
 *
 * The per-MR gate replaces a task-level `auto_fix_enabled` check: now that
 * the switches are per MR, that top-level boolean is an aggregate over every
 * linked MR, so gating on it hid the round counter entirely whenever
 * auto-fix was on for some linked MRs but not all.
 */
export function selectBadgeMR(
  openMRs: TaskMR[],
  options: TaskMRAutomationOptions | null,
): TaskMR | null {
  let best: TaskMR | null = null;
  let bestRound: AutoFixRoundInfo | null = null;
  for (const mr of openMRs) {
    if (!switchesForMR(options, mr).autoFix) continue;
    const round = autoFixRoundFor(options, mr);
    if (!best || !bestRound || isBadgeMRBetter(round, mr, bestRound, best)) {
      best = mr;
      bestRound = round;
    }
  }
  return best;
}

/**
 * Automation flags + round info for the chip's badges, live (never frozen).
 * A badge lights up when *any* open MR has that switch on, mirroring
 * GitHub's automationForPRs — the task-level booleans read "on" only when
 * every linked MR has the switch on, which would leave the chip silently
 * under-reporting an armed auto-merge.
 */
export function chipAutomation(
  options: TaskMRAutomationOptions | null,
  openMRs: TaskMR[],
): ChipAutomation {
  const badgeMR = selectBadgeMR(openMRs, options);
  const perMR = openMRs.map((mr) => switchesForMR(options, mr));
  return {
    autoFixEnabled: perMR.some((s) => s.autoFix),
    autoMergeEnabled: perMR.some((s) => s.autoMerge),
    autoFixRound: badgeMR ? autoFixRoundFor(options, badgeMR) : null,
  };
}

/**
 * Freezes which MR the disclosure acts on by association `id` while it is
 * open (spec: "Freezing while the disclosure is open"). Captures `id` only,
 * never a copy of the TaskMR object, so the returned `actedOnMR` always
 * re-reads the live store row for that id. Deliberately local to whichever
 * disclosure variant (popover vs drawer) calls it: a pointer-precision
 * change unmounts one variant and mounts the other, so this state resets to
 * unfrozen on that transition with no extra code (spec round-4 finding
 * F31) — the freeze can never survive a variant swap because there is
 * nothing above the swap point holding it.
 */
export function useFrozenChipSelection(mrs: TaskMR[], liveSelected: TaskMR, open: boolean) {
  const [frozenId, setFrozenId] = useState<string | null>(null);
  const wasOpen = useRef(false);

  useEffect(() => {
    if (open && !wasOpen.current) setFrozenId(liveSelected.id);
    if (!open) setFrozenId(null);
    wasOpen.current = open;
  }, [open, liveSelected.id]);

  const frozenMR = frozenId
    ? (mrs.find((mr) => mr.id === frozenId && mr.state === "open") ?? null)
    : null;
  // If the held id leaves the store, or its row stops being open, the
  // disclosure closes (spec). The caller effects on `shouldForceClose`.
  const shouldForceClose = frozenId !== null && frozenMR === null;
  const actedOnMR = frozenMR ?? liveSelected;
  return { actedOnMR, frozen: frozenId !== null && !shouldForceClose, shouldForceClose };
}

/**
 * Radix treats the trigger as outside the popover content, so a click on
 * the chip itself would otherwise close it via onPointerDownOutside. This
 * guard filters out trigger clicks so clicking the chip is a no-op while
 * the popover stays open (spec: "Clicking the fine-pointer trigger is a
 * no-op in both states").
 */
export function useTriggerOutsideGuard() {
  const ref = useRef<HTMLButtonElement>(null);
  const onPointerDownOutside = useCallback(
    (e: { target: EventTarget | null; preventDefault: () => void }) => {
      if (ref.current && ref.current.contains(e.target as Node)) e.preventDefault();
    },
    [],
  );
  return { ref, onPointerDownOutside };
}
