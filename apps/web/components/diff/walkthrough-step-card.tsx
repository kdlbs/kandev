"use client";

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import { IconX, IconFileCode } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import { revealFileAtLine, type OpenFileFn } from "@/lib/diff/walkthrough-reveal";
import { generateUUID } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  useActiveTaskWalkthrough,
  useTaskWalkthrough,
  useWalkthroughSeen,
  useWalkthroughStepState,
} from "@/hooks/domains/session/use-task-walkthrough";
import {
  markdownComponents,
  normalizeMarkdown,
  remarkPlugins,
} from "@/components/shared/markdown-components";
import type { TaskWalkthrough, WalkthroughStep } from "@/lib/types/http";
import type { WalkthroughComment } from "@/lib/state/slices/comments";
import { CommentForm } from "./comment-form";

export const WALKTHROUGH_STEP_BODY_CLASS =
  "prose prose-sm dark:prose-invert max-w-none text-left text-sm leading-6 [overflow-wrap:anywhere] [text-align:left] [&_li]:text-left [&_p]:my-2 [&_p]:text-left";

function StepBody({ step, onOpenFile }: { step: WalkthroughStep; onOpenFile: () => void }) {
  const lineLabel = step.line_end ? `${step.line}–${step.line_end}` : `${step.line}`;
  const fileName = step.file.split("/").pop() || step.file;
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
      {step.title ? (
        <h4 className="text-sm font-semibold mb-1.5" data-testid="walkthrough-step-title">
          {step.title}
        </h4>
      ) : null}
      <button
        type="button"
        onClick={onOpenFile}
        title={`Open ${step.file}:${step.line}`}
        data-testid="walkthrough-step-file"
        className="mb-2.5 flex max-w-full cursor-pointer items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium hover:bg-muted"
      >
        <IconFileCode className="size-3.5 shrink-0 text-primary" />
        <span className="truncate font-mono">{fileName}</span>
        <span className="shrink-0 text-muted-foreground">:{lineLabel}</span>
      </button>
      <div className={WALKTHROUGH_STEP_BODY_CLASS} data-testid="walkthrough-step-body">
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
    <div
      className="flex cursor-move touch-none items-center gap-2 border-b border-border px-3 pt-2 pb-1.5"
      data-testid="walkthrough-drag-handle"
      data-walkthrough-drag-handle
    >
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

function StepNavigation({
  activeStep,
  stepCount,
  onPrevious,
  onNext,
}: {
  activeStep: number;
  stepCount: number;
  onPrevious: () => void;
  onNext: () => void;
}) {
  return (
    <div className="flex items-center gap-2 px-4 py-2 border-t border-border">
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer"
        disabled={activeStep <= 0}
        data-testid="walkthrough-prev"
        onClick={onPrevious}
      >
        Previous
      </Button>
      <Button
        size="sm"
        className="cursor-pointer"
        disabled={activeStep >= stepCount - 1}
        data-testid="walkthrough-next"
        onClick={onNext}
      >
        Next
      </Button>
    </div>
  );
}

/** True when the active task has a walkthrough with a valid active step. */
export function useHasActiveWalkthroughStep(): boolean {
  const { taskId, data: walkthrough } = useActiveTaskWalkthrough();
  const activeStep = useAppStore((s) =>
    taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0,
  );
  return Boolean(walkthrough?.steps[activeStep]);
}

function useWalkthroughStepFeedback(params: {
  activeStep: number;
  activeTaskId: string | null;
  sessionId: string | null | undefined;
  step: WalkthroughStep | undefined;
  stepCount: number;
  walkthrough: TaskWalkthrough | null | undefined;
}) {
  const { activeStep, activeTaskId, sessionId, step, stepCount, walkthrough } = params;
  const addComment = useCommentsStore((s) => s.addComment);
  const { runComment } = useRunComment({ sessionId: sessionId ?? null, taskId: activeTaskId });
  const { toast } = useToast();
  const buildComment = (text: string): WalkthroughComment | null => {
    if (!sessionId || !activeTaskId || !walkthrough || !step) return null;
    return buildWalkthroughComment({
      sessionId,
      taskId: activeTaskId,
      walkthrough,
      step,
      activeStep,
      stepCount,
      text,
    });
  };
  const showMissingSessionError = () => {
    toast({ title: "No active session for walkthrough note", variant: "error" });
  };
  const addWalkthroughFeedback = (text: string) => {
    const comment = buildComment(text);
    if (!comment) {
      showMissingSessionError();
      return;
    }
    addComment(comment);
    toast({ title: "Walkthrough note added", variant: "success" });
  };
  const runWalkthroughFeedback = (text: string) => {
    const comment = buildComment(text);
    if (!comment) {
      showMissingSessionError();
      return;
    }
    addComment(comment);
    void runComment(comment)
      .then(({ queued }) => {
        toast({
          title: queued ? "Walkthrough note queued" : "Walkthrough note sent",
          variant: "success",
        });
      })
      .catch(() => {
        toast({ title: "Failed to send walkthrough note", variant: "error" });
      });
  };
  return { addWalkthroughFeedback, runWalkthroughFeedback };
}

/**
 * The shared inner card body (header, markdown, Prev/Next, ask box) used by both
 * the inline diff-anchored card and the editor-mode floating window. Reads all
 * state from the store; renders nothing when there is no active step. The note
 * box mirrors other agent feedback surfaces: "Add" creates a pending
 * walkthrough context item, and "Run" sends the same context to the agent.
 */
export function WalkthroughStepInner({
  onClose,
  onSelectFile,
}: {
  onClose: () => void;
  onSelectFile?: OpenFileFn;
}) {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const walkthrough = useTaskWalkthrough(activeTaskId).data ?? null;
  const stepCount = walkthrough?.steps.length ?? 0;
  const { activeStep, setActiveStep } = useWalkthroughStepState(activeTaskId, stepCount);
  const { markSeen } = useWalkthroughSeen(activeTaskId, walkthrough);
  const { openFile: defaultOpenFile } = useFileEditors();
  const openFile = onSelectFile ?? defaultOpenFile;
  const step = walkthrough?.steps[activeStep];
  const { addWalkthroughFeedback, runWalkthroughFeedback } = useWalkthroughStepFeedback({
    activeStep,
    activeTaskId,
    sessionId,
    step,
    stepCount,
    walkthrough,
  });
  useEffect(() => {
    if (activeTaskId && walkthrough) markSeen();
  }, [activeTaskId, walkthrough, markSeen]);
  if (!activeTaskId || !walkthrough) return null;
  if (!step) return null;
  const lineLabel = step.line_end ? `Lines ${step.line}–${step.line_end}` : `Line ${step.line}`;

  return (
    <div className="flex max-h-[calc(100dvh-2rem)] flex-col rounded-xl border-l-2 border-primary/60 border border-border bg-card shadow-lg sm:max-h-[min(78vh,720px)]">
      <StepHeader
        activeStep={activeStep}
        stepCount={stepCount}
        lineLabel={lineLabel}
        onClose={onClose}
      />
      <StepBody
        step={step}
        onOpenFile={() => revealFileAtLine(openFile, step.file, step.line, step.repo)}
      />
      <StepNavigation
        activeStep={activeStep}
        stepCount={stepCount}
        onPrevious={() => setActiveStep(activeStep - 1)}
        onNext={() => setActiveStep(activeStep + 1)}
      />
      <div className="px-4 pb-3 pt-1">
        <CommentForm
          key={`${walkthrough.id}:${activeStep}`}
          onSubmit={addWalkthroughFeedback}
          onSubmitAndRun={runWalkthroughFeedback}
          onCancel={() => {}}
          autoFocus={false}
        />
      </div>
    </div>
  );
}

function buildWalkthroughComment(params: {
  sessionId: string;
  taskId: string;
  walkthrough: TaskWalkthrough;
  step: WalkthroughStep;
  activeStep: number;
  stepCount: number;
  text: string;
}): WalkthroughComment {
  const { sessionId, taskId, walkthrough, step, activeStep, stepCount, text } = params;
  const startLine = step.line;
  const endLine = step.line_end ?? step.line;
  return {
    id: generateUUID(),
    source: "walkthrough",
    sessionId,
    taskId,
    walkthroughId: walkthrough.id,
    walkthroughTitle: walkthrough.title,
    stepIndex: activeStep,
    stepCount,
    repo: step.repo,
    filePath: step.file,
    startLine: Math.min(startLine, endLine),
    endLine: Math.max(startLine, endLine),
    stepText: step.text,
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

/**
 * Inline walkthrough popover rendered as a diff annotation, anchored directly
 * beneath the step's line. A caret + left accent visually bind it to the line
 * above.
 */
export function WalkthroughStepCard() {
  const hasStep = useHasActiveWalkthroughStep();
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const walkthrough = useTaskWalkthrough(activeTaskId).data ?? null;
  const activeStep = useAppStore((s) =>
    activeTaskId ? (s.walkthroughs.activeStepByTaskId[activeTaskId] ?? 0) : 0,
  );
  const updatedAt = walkthrough?.updated_at;
  const [dismissedKey, setDismissedKey] = useState<string | null>(null);
  const stepKey = activeTaskId ? `${activeTaskId}:${updatedAt ?? ""}:${activeStep}` : "";
  if (!hasStep || dismissedKey === stepKey) return null;
  return (
    <div className="relative my-2 ml-2 mr-2" data-testid="walkthrough-overlay">
      <div className="absolute -top-1.5 left-6 size-3 rotate-45 border-l border-t border-primary/60 bg-card" />
      <WalkthroughStepInner onClose={() => setDismissedKey(stepKey)} />
    </div>
  );
}
