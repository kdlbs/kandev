"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { revealFileAtLine } from "@/lib/diff/walkthrough-reveal";
import { WalkthroughStepInner, useHasActiveWalkthroughStep } from "./walkthrough-step-card";

/**
 * The primary walkthrough surface: a floating card that, for each step, opens the
 * step's file in its *current state* and reveals/centers the anchored line in the
 * editor. Works uniformly for changed and unchanged files (onboarding / whole-repo
 * tours), so it does not depend on the file being part of a diff.
 */
export function WalkthroughFloatingWindow({ onClose }: { onClose: () => void }) {
  const hasStep = useHasActiveWalkthroughStep();
  const { openFile } = useFileEditors();
  // Select primitives (not a fresh object) so the selector stays referentially
  // stable — returning a new object here would loop the store subscription.
  const stepFile = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.file ?? null;
  });
  const stepLine = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return 0;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.line ?? 0;
  });
  const stepRepo = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return undefined;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.repo;
  });

  // Open the step's file (current state) and reveal its line whenever the step changes.
  useEffect(() => {
    if (stepFile) revealFileAtLine(openFile, stepFile, stepLine, stepRepo);
  }, [stepFile, stepLine, stepRepo, openFile]);

  if (!hasStep) return null;

  return (
    <div
      data-testid="walkthrough-floating"
      className="fixed right-6 top-20 z-[70] w-[400px] max-w-[calc(100vw-2rem)]"
    >
      <WalkthroughStepInner onClose={onClose} />
    </div>
  );
}
