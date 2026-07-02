"use client";

import { useEffect } from "react";
import ReactMarkdown from "react-markdown";
import { IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { buildDiffComment } from "@/lib/diff/comment-utils";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  markdownComponents,
  normalizeMarkdown,
  remarkPlugins,
} from "@/components/shared/markdown-components";
import type { WalkthroughStep } from "@/lib/types/http";
import { CommentForm } from "./comment-form";

/** Compose the chat prompt sent when the user asks about a step. */
function buildStepQuestion(step: WalkthroughStep, stepIndex: number, question: string): string {
  const where = step.line_end
    ? `${step.file}:${step.line}-${step.line_end}`
    : `${step.file}:${step.line}`;
  return (
    `<kandev-system>Re: walkthrough step ${stepIndex + 1} (${where})</kandev-system>\n\n` +
    `> ${step.text}\n\n${question}`
  );
}

function StepBody({ step }: { step: WalkthroughStep }) {
  return (
    <div className="px-3 py-2 max-h-[40vh] overflow-y-auto">
      {step.title ? (
        <h4 className="text-sm font-semibold mb-1" data-testid="walkthrough-step-title">
          {step.title}
        </h4>
      ) : null}
      <div
        className="prose prose-sm dark:prose-invert max-w-none text-sm"
        data-testid="walkthrough-step-body"
      >
        <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
          {normalizeMarkdown(step.text)}
        </ReactMarkdown>
      </div>
    </div>
  );
}

function StepHeader({
  activeStep,
  stepCount,
  lineLabel,
  onClose,
}: {
  activeStep: number;
  stepCount: number;
  lineLabel: string;
  onClose: () => void;
}) {
  return (
    <div className="flex items-center gap-2 px-3 pt-2 pb-1.5 border-b border-border">
      <Badge variant="secondary" data-testid="walkthrough-badge">
        Walkthrough
      </Badge>
      <span className="text-xs text-muted-foreground" data-testid="walkthrough-step-header">
        Step {activeStep + 1} / {stepCount} · {lineLabel}
      </span>
      <Button
        variant="ghost"
        size="icon"
        className="ml-auto size-6 cursor-pointer"
        aria-label="Close walkthrough"
        data-testid="walkthrough-close"
        onClick={onClose}
      >
        <IconX className="size-4" />
      </Button>
    </div>
  );
}

/** True when the active task has a walkthrough with a valid active step. */
export function useHasActiveWalkthroughStep(): boolean {
  return useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return false;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return !!wt?.steps[idx];
  });
}

/**
 * The shared inner card body (header, markdown, Prev/Next, ask box) used by both
 * the inline diff-anchored card and the editor-mode floating window. Reads all
 * state from the store; renders nothing when there is no active step. The ask
 * box mirrors diff comments: "Add" queues a review comment on these lines,
 * "Run" sends the question to the agent immediately.
 */
export function WalkthroughStepInner() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const walkthrough = useAppStore((s) =>
    activeTaskId ? s.walkthroughs.byTaskId[activeTaskId] : null,
  );
  const activeStep = useAppStore((s) =>
    activeTaskId ? (s.walkthroughs.activeStepByTaskId[activeTaskId] ?? 0) : 0,
  );
  const setActiveStep = useAppStore((s) => s.setWalkthroughActiveStep);
  const setWalkthrough = useAppStore((s) => s.setWalkthrough);
  const markSeen = useAppStore((s) => s.markWalkthroughSeen);
  const addComment = useCommentsStore((s) => s.addComment);

  useEffect(() => {
    if (activeTaskId && walkthrough) markSeen(activeTaskId);
  }, [activeTaskId, walkthrough, markSeen]);

  if (!activeTaskId || !walkthrough) return null;
  const stepCount = walkthrough.steps.length;
  const step = walkthrough.steps[activeStep];
  if (!step) return null;

  const lineLabel = step.line_end ? `Lines ${step.line}–${step.line_end}` : `Line ${step.line}`;

  const addAsComment = (text: string) => {
    if (!sessionId) return;
    addComment(
      buildDiffComment({
        filePath: step.file,
        sessionId,
        startLine: step.line,
        endLine: step.line_end ?? step.line,
        side: "additions",
        text,
      }),
    );
  };

  const askNow = (text: string) => {
    if (!sessionId) return;
    getWebSocketClient()
      ?.request(
        "message.add",
        {
          task_id: activeTaskId,
          session_id: sessionId,
          content: buildStepQuestion(step, activeStep, text),
        },
        10000,
      )
      .catch(() => {});
  };

  return (
    <div className="rounded-xl border-l-2 border-primary/60 border border-border bg-card shadow-lg">
      <StepHeader
        activeStep={activeStep}
        stepCount={stepCount}
        lineLabel={lineLabel}
        onClose={() => setWalkthrough(activeTaskId, null)}
      />
      <StepBody step={step} />
      <div className="flex items-center gap-2 px-3 py-1.5 border-t border-border">
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={activeStep <= 0}
          data-testid="walkthrough-prev"
          onClick={() => setActiveStep(activeTaskId, activeStep - 1)}
        >
          Previous
        </Button>
        <Button
          size="sm"
          className="cursor-pointer"
          disabled={activeStep >= stepCount - 1}
          data-testid="walkthrough-next"
          onClick={() => setActiveStep(activeTaskId, activeStep + 1)}
        >
          Next
        </Button>
      </div>
      <div className="px-3 pb-2 pt-1">
        <CommentForm
          key={activeStep}
          onSubmit={addAsComment}
          onSubmitAndRun={askNow}
          onCancel={() => {}}
          autoFocus={false}
        />
      </div>
    </div>
  );
}

/**
 * Inline walkthrough popover rendered as a diff annotation, anchored directly
 * beneath the step's line. A caret + left accent visually bind it to the line
 * above.
 */
export function WalkthroughStepCard() {
  const hasStep = useHasActiveWalkthroughStep();
  if (!hasStep) return null;
  return (
    <div className="relative my-2 ml-2 mr-2" data-testid="walkthrough-overlay">
      <div className="absolute -top-1.5 left-6 size-3 rotate-45 border-l border-t border-primary/60 bg-card" />
      <WalkthroughStepInner />
    </div>
  );
}
