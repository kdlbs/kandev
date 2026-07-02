"use client";

import { useEffect } from "react";
import { IconMinus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { WalkthroughStepInner, useHasActiveWalkthroughStep } from "./walkthrough-step-card";

type WalkthroughFloatingWindowProps = {
  /** Opens a file's current state in an editor tab. */
  openFile: (path: string, repo?: string) => void | Promise<void>;
  /** Exit editor mode (hide the floating window) without dismissing the tour. */
  onExit: () => void;
};

/**
 * Editor-mode walkthrough: a floating window over the editor tabs that opens the
 * active step's file in its *current state* (via openFile) and shows the step
 * card. Unlike the diff-anchored card, this works for files that did not change,
 * enabling onboarding / whole-repo tours.
 */
export function WalkthroughFloatingWindow({ openFile, onExit }: WalkthroughFloatingWindowProps) {
  const hasStep = useHasActiveWalkthroughStep();
  // Select primitives (not a fresh object) so the selector stays referentially
  // stable — returning a new object here would loop the store subscription.
  const stepFile = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.file ?? null;
  });
  const stepRepo = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return undefined;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.repo;
  });

  // Open the step's file (current state) in a tab whenever the step changes.
  useEffect(() => {
    if (stepFile) void openFile(stepFile, stepRepo);
  }, [stepFile, stepRepo, openFile]);

  if (!hasStep) return null;

  return (
    <div
      data-testid="walkthrough-floating"
      className="fixed right-6 top-20 z-[70] w-[380px] max-w-[calc(100vw-2rem)]"
    >
      <div className="mb-1 flex items-center justify-between rounded-md bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
        Editor walkthrough
        <Button
          variant="ghost"
          size="icon"
          className="size-5 cursor-pointer text-primary"
          aria-label="Exit editor walkthrough"
          data-testid="walkthrough-floating-exit"
          onClick={onExit}
        >
          <IconMinus className="size-3.5" />
        </Button>
      </div>
      <WalkthroughStepInner />
    </div>
  );
}
