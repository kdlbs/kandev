import {
  walkthroughStepMatchesFile,
  type WalkthroughTargetFile,
} from "@/lib/diff/walkthrough-match";
import type { WalkthroughStep } from "@/lib/types/http";

export type WalkthroughEditorRange = {
  startLine: number;
  endLine: number;
};

export type WalkthroughRangeBox = WalkthroughEditorRange & {
  top: number;
  left: number;
  width: number;
  height: number;
};

export function getWalkthroughEditorRange(
  file: WalkthroughTargetFile,
  step: WalkthroughStep | null | undefined,
): WalkthroughEditorRange | null {
  if (!step || !walkthroughStepMatchesFile(file, step)) return null;
  return {
    startLine: Math.min(step.line, step.line_end ?? step.line),
    endLine: Math.max(step.line, step.line_end ?? step.line),
  };
}
