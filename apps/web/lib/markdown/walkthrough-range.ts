import type { WalkthroughEditorRange } from "@/lib/walkthrough-editor-range";

export type MarkdownWalkthroughTarget = WalkthroughEditorRange & {
  key: string;
  path: string;
  repositoryName?: string;
  taskId: string;
  sessionId?: string;
  stepIndex: number;
  navigationLine?: number;
  file: string;
  repo?: string;
};

export const WALKTHROUGH_RANGE_CLASS =
  "pointer-events-none absolute z-20 rounded-sm border-l-2 border-primary/70 bg-primary/10 shadow-[inset_0_0_0_1px_hsl(var(--primary)/0.18)]";
