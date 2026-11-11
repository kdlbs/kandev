"use client";

import { useEffect, useMemo, useState } from "react";
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
  markdownComponents,
  normalizeMarkdown,
  rehypePlugins,
  remarkPlugins,
} from "@/components/shared/markdown-components";
import type { TaskWalkthrough, WalkthroughStep } from "@/lib/types/http";
import type { WalkthroughComment } from "@/lib/state/slices/comments";
import { WalkthroughCommentList, WalkthroughStepFeedback } from "./walkthrough-step-feedback";
import { WalkthroughStepPicker } from "./walkthrough-step-picker";
import { WalkthroughGenerationTime } from "./walkthrough-generation-time";
import { useWalkthroughTap } from "./use-walkthrough-tap";
import { getWalkthroughComments } from "@/lib/walkthrough-comments";
import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";

export const WALKTHROUGH_STEP_BODY_CLASS =
  "prose prose-sm dark:prose-invert max-w-none text-left text-sm leading-6 [overflow-wrap:anywhere] [text-align:left] [&_li]:text-left [&_p]:my-2 [&_p]:text-left";

function StepBody({
  step,
  comments,
  onOpenFile,
}: {
  step: WalkthroughStep;
  comments: WalkthroughComment[];
  onOpenFile: () => void;
}) {
  const { t } = useTranslation();
  const lineLabel = step.line_end ? `${step.line}–${step.line_end}` : `${step.line}`;
  const fileName = step.file.split("/").pop() || step.file;
  return (
    <div
      className="min-h-0 flex-auto overflow-y-auto overscroll-contain"
      data-testid="walkthrough-step-content"
    >
      <div className="px-4 py-3">
        {step.title ? (
          <h4 className="text-sm font-semibold mb-1.5" data-testid="walkthrough-step-title">
            {step.title}
          </h4>
        ) : null}
        <button
          type="button"
          onClick={onOpenFile}
          title={t("diff:openFileAtLine", { file: step.file, line: step.line })}
          data-testid="walkthrough-step-file"
          className="mb-2.5 flex max-w-full cursor-pointer items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium hover:bg-muted"
        >
          <IconFileCode className="size-3.5 shrink-0 text-primary" />
          <span className="truncate font-mono">{fileName}</span>
          <span className="shrink-0 text-muted-foreground">:{lineLabel}</span>
        </button>
        <div className={WALKTHROUGH_STEP_BODY_CLASS} data-testid="walkthrough-step-body">
          <ReactMarkdown
            remarkPlugins={remarkPlugins}
            rehypePlugins={rehypePlugins}
            components={markdownComponents}
          >
            {normalizeMarkdown(step.text)}
          </ReactMarkdown>
        </div>
        <WalkthroughCommentList comments={comments} />
      </div>
    </div>
  );
}

function StepHeader({
  activeStep,
  stepCount,
  step,
  updatedAt,
  onClose,
}: {
  activeStep: number;
  stepCount: number;
  step: WalkthroughStep;
  updatedAt: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const lineLabel = step.line_end
    ? t("diff:lineRangeLabel", { start: step.line, end: step.line_end })
    : t("diff:lineLabel", { line: step.line });
  return (
    <div className="flex shrink-0 items-center gap-2 border-b border-border px-3 pt-2 pb-1.5">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div
          className="flex cursor-move touch-none flex-wrap items-center gap-x-2"
          data-testid="walkthrough-drag-handle"
          data-walkthrough-drag-handle
        >
          <Badge variant="secondary" data-testid="walkthrough-badge">
            {t("diff:walkthrough")}
          </Badge>
          <span className="text-xs text-muted-foreground" data-testid="walkthrough-step-header">
            {t("diff:stepProgress", { current: activeStep + 1, total: stepCount, lineLabel })}
          </span>
        </div>
        <WalkthroughGenerationTime updatedAt={updatedAt} />
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="ml-auto size-7 shrink-0 cursor-pointer max-md:size-11 pointer-coarse:size-11"
        aria-label={t("diff:closeWalkthrough")}
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
  const { t } = useTranslation();
  const tapHandlers = useWalkthroughTap();
  return (
    <div
      className="flex shrink-0 items-center gap-2 border-t border-border px-4 py-2"
      data-testid="walkthrough-navigation"
    >
      <Button
        variant="outline"
        size="sm"
        className="h-7 cursor-pointer max-md:h-11 pointer-coarse:h-11"
        disabled={activeStep <= 0}
        data-testid="walkthrough-prev"
        onClick={onPrevious}
        {...(activeStep > 0 ? tapHandlers(onPrevious) : {})}
      >
        {t("diff:previous")}
      </Button>
      <Button
        size="sm"
        className="h-7 cursor-pointer max-md:h-11 pointer-coarse:h-11"
        disabled={activeStep >= stepCount - 1}
        data-testid="walkthrough-next"
        onClick={onNext}
        {...(activeStep < stepCount - 1 ? tapHandlers(onNext) : {})}
      >
        {t("diff:next")}
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

function useWalkthroughStepFeedback(params: {
  activeStep: number;
  activeTaskId: string | null;
  sessionId: string | null | undefined;
  step: WalkthroughStep | undefined;
  stepCount: number;
  walkthrough: TaskWalkthrough | null | undefined;
}) {
  const { t } = useTranslation();
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
    toast({ title: t("diff:noActiveSessionForWalkthroughNote"), variant: "error" });
  };
  const addWalkthroughFeedback = (text: string) => {
    const comment = buildComment(text);
    if (!comment) {
      showMissingSessionError();
      return false;
    }
    addComment(comment);
    toast({ title: t("diff:walkthroughNoteAdded"), variant: "success" });
    return true;
  };
  const runWalkthroughFeedback = (text: string) => {
    const comment = buildComment(text);
    if (!comment) {
      showMissingSessionError();
      return false;
    }
    addComment(comment);
    void runComment(comment)
      .then(({ queued }) => {
        toast({
          title: queued ? t("diff:walkthroughNoteQueued") : t("diff:walkthroughNoteSent"),
          variant: "success",
        });
      })
      .catch(() => {
        toast({ title: t("diff:failedToSendWalkthroughNote"), variant: "error" });
      });
    return true;
  };
  return { addWalkthroughFeedback, runWalkthroughFeedback };
}

/**
 * The shared inner card body (header, markdown, Prev/Next, ask box) used by both
 * the inline diff-anchored card and the editor-mode floating window. Reads all
 * state from the store; renders nothing when there is no active step. Feedback
 * follows other agent feedback surfaces: "Add" creates a pending
 * walkthrough context item, and "Run" sends the same context to the agent.
 */
export function WalkthroughStepInner({
  onClose,
  onSelectFile,
  layout = "inline",
}: {
  onClose: () => void;
  onSelectFile?: OpenFileFn;
  layout?: "inline" | "floating";
}) {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const walkthrough = useAppStore((s) =>
    activeTaskId ? s.walkthroughs.byTaskId[activeTaskId] : null,
  );
  const activeStep = useAppStore((s) =>
    activeTaskId ? (s.walkthroughs.activeStepByTaskId[activeTaskId] ?? 0) : 0,
  );
  const setActiveStep = useAppStore((s) => s.setWalkthroughActiveStep);
  const markSeen = useAppStore((s) => s.markWalkthroughSeen);
  const { openFile: defaultOpenFile } = useFileEditors();
  const openFile = onSelectFile ?? defaultOpenFile;
  const stepCount = walkthrough?.steps.length ?? 0;
  const step = walkthrough?.steps[activeStep];
  const { addWalkthroughFeedback, runWalkthroughFeedback } = useWalkthroughStepFeedback({
    activeStep,
    activeTaskId,
    sessionId,
    step,
    stepCount,
    walkthrough,
  });
  const byId = useCommentsStore((s) => s.byId);
  const hydrateSession = useCommentsStore((s) => s.hydrateSession);
  const commentsByStep = useMemo(
    () =>
      walkthrough && sessionId
        ? walkthrough.steps.map((_, index) =>
            getWalkthroughComments(Object.values(byId), walkthrough, sessionId, index),
          )
        : [],
    [byId, walkthrough, sessionId],
  );
  useEffect(() => {
    if (sessionId) hydrateSession(sessionId);
  }, [sessionId, hydrateSession]);
  useEffect(() => {
    if (activeTaskId && walkthrough) markSeen(activeTaskId);
  }, [activeTaskId, walkthrough, markSeen]);
  if (!activeTaskId || !walkthrough || !step) return null;
  const navigation = (
    <StepNavigation
      activeStep={activeStep}
      stepCount={stepCount}
      onPrevious={() => setActiveStep(activeTaskId, activeStep - 1)}
      onNext={() => setActiveStep(activeTaskId, activeStep + 1)}
    />
  );

  return (
    <div
      className={cn(
        "flex max-h-[calc(100dvh-2rem)] flex-col rounded-xl border border-border border-l-2 border-primary/60 bg-card shadow-lg md:max-h-[min(78vh,720px)]",
        layout === "floating" &&
          "max-h-[calc(100dvh-1rem-var(--app-status-bar-height)-env(safe-area-inset-bottom,0px))] overflow-hidden md:max-h-[min(78dvh,720px)]",
      )}
    >
      <StepHeader
        activeStep={activeStep}
        stepCount={stepCount}
        step={step}
        updatedAt={walkthrough.updated_at}
        onClose={onClose}
      />
      <WalkthroughStepPicker
        walkthrough={walkthrough}
        activeStep={activeStep}
        counts={commentsByStep.map((comments) => comments.length)}
        onSelect={(index) => setActiveStep(activeTaskId, index)}
      />
      <StepBody
        step={step}
        comments={commentsByStep[activeStep] ?? []}
        onOpenFile={() => revealFileAtLine(openFile, step.file, step.line, step.repo)}
      />
      <WalkthroughStepFeedback
        key={`${activeTaskId}:${sessionId}:${walkthrough.id}:${walkthrough.updated_at}`}
        walkthrough={walkthrough}
        sessionId={sessionId ?? null}
        activeStep={activeStep}
        commentCount={commentsByStep[activeStep]?.length ?? 0}
        onSubmit={addWalkthroughFeedback}
        onRun={runWalkthroughFeedback}
      />
      {navigation}
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
    walkthroughUpdatedAt: walkthrough.updated_at,
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
  const activeStep = useAppStore((s) =>
    activeTaskId ? (s.walkthroughs.activeStepByTaskId[activeTaskId] ?? 0) : 0,
  );
  const updatedAt = useAppStore((s) =>
    activeTaskId ? s.walkthroughs.byTaskId[activeTaskId]?.updated_at : undefined,
  );
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
