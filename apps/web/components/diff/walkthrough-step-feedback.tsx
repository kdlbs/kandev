"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { TaskWalkthrough } from "@/lib/types/http";
import type { WalkthroughComment } from "@/lib/state/slices/comments";
import { useWalkthroughDrafts, walkthroughDraftKey } from "@/lib/walkthrough-feedback-state";
import { CommentForm } from "./comment-form";

export type WalkthroughStepFeedbackProps = {
  walkthrough: TaskWalkthrough;
  sessionId: string | null;
  activeStep: number;
  commentCount: number;
  onSubmit: (text: string) => boolean;
  onRun: (text: string) => boolean;
};

export function WalkthroughCommentList({ comments }: { comments: WalkthroughComment[] }) {
  const { t } = useTranslation();
  if (!comments.length) return null;
  return (
    <section className="mt-4 border-t border-border pt-3" data-testid="walkthrough-comments">
      <h5 className="mb-2 text-xs font-medium text-muted-foreground">
        {t("diff:walkthroughCommentCount", { count: comments.length })}
      </h5>
      <ul className="divide-y divide-border">
        {comments.map((comment) => (
          <li
            key={comment.id}
            className="py-2"
            data-testid="walkthrough-comment"
            data-status={comment.status}
          >
            <p className="whitespace-pre-wrap text-sm [overflow-wrap:anywhere]">{comment.text}</p>
            <span className="mt-1 block text-xs text-muted-foreground">
              {comment.status === "sent"
                ? t("diff:walkthroughSubmitted")
                : t("diff:walkthroughPending")}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function StepComposer({
  walkthrough,
  sessionId,
  activeStep,
  commentCount,
  onSubmit,
  onRun,
}: WalkthroughStepFeedbackProps) {
  const { t } = useTranslation();
  const key = walkthroughDraftKey(sessionId ?? "", walkthrough, activeStep);
  const draft = useWalkthroughDrafts((state) => state.drafts[key]?.text ?? "");
  const setDraft = useWalkthroughDrafts((state) => state.setDraft);
  const [openKey, setOpenKey] = useState<string | null>(null);
  const open = openKey === key;
  const trigger = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  const composerId = useId();
  useEffect(() => {
    if (!open && restoreFocus.current) {
      trigger.current?.focus();
      restoreFocus.current = false;
    }
  }, [open]);
  const close = () => {
    restoreFocus.current = true;
    setOpenKey(null);
  };
  const updateDraft = (text: string) => setDraft(sessionId ?? "", walkthrough, activeStep, text);
  const save = (text: string, run: boolean) => {
    if (!(run ? onRun(text) : onSubmit(text))) return false;
    updateDraft("");
    close();
    return true;
  };
  return (
    // The minimum reserves a 44px input, 44px actions, and their spacing.
    <div className={open ? "flex min-h-[8.125rem] flex-col px-4 py-2" : "shrink-0 px-4 py-2"}>
      <Button
        ref={trigger}
        variant="outline"
        hidden={open}
        aria-label={t("diff:walkthroughComment")}
        aria-expanded={open}
        aria-controls={composerId}
        data-testid="walkthrough-comment-trigger"
        className="h-7 cursor-pointer max-md:h-11 pointer-coarse:h-11"
        onClick={() => setOpenKey(key)}
      >
        <IconMessageCircle className="size-4" />
        {t("diff:walkthroughComment")}
        <span className="tabular-nums text-muted-foreground">({commentCount})</span>
      </Button>
      {open && (
        <div id={composerId} className="flex min-h-0 flex-col">
          <CommentForm
            value={draft}
            onChange={updateDraft}
            onSubmit={(text) => save(text, false)}
            onSubmitAndRun={(text) => save(text, true)}
            onCancel={close}
            className="min-h-0 shadow-none [&_textarea]:min-h-11 [&_textarea]:max-h-[min(9rem,25dvh)] [&_textarea]:overflow-y-auto [&_[data-comment-actions]]:shrink-0 [&_button]:h-7 max-md:[&_button]:h-11 pointer-coarse:[&_button]:h-11 max-md:[&_[data-comment-shortcuts]]:hidden pointer-coarse:[&_[data-comment-shortcuts]]:hidden"
          />
        </div>
      )}
    </div>
  );
}

export function WalkthroughStepFeedback(props: WalkthroughStepFeedbackProps) {
  const key = walkthroughDraftKey(props.sessionId ?? "", props.walkthrough, props.activeStep);
  return <StepComposer key={key} {...props} />;
}
