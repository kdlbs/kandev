"use client";

import { IconMessagePlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import { cn } from "@/lib/utils";

type PreviewFeedbackControlsProps = {
  capture: PreviewCaptureController;
  enabled: boolean;
};

/** Header toggle for annotation mode, with the pending count and an unsaved-draft dot. */
export function PreviewFeedbackControls({ capture, enabled }: PreviewFeedbackControlsProps) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const active = capture.annotation.active;
  const count = capture.items.length;
  const unsaved = capture.drafts.length;
  const label = t("task:previewAnnotateWithCount", { count });
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant={active ? "default" : "ghost"}
        disabled={!enabled}
        aria-pressed={active}
        aria-label={label}
        title={label}
        onClick={capture.annotation.toggle}
        className={cn(
          "relative cursor-pointer gap-1 text-xs",
          touch ? "h-11 min-w-11 px-2" : "h-7 min-w-7 px-1.5",
        )}
        data-testid="preview-feedback-trigger"
        data-annotation-active={active ? "true" : "false"}
        data-capture-mode={capture.acknowledgedMode ?? "none"}
      >
        <IconMessagePlus className="h-4 w-4 shrink-0" aria-hidden="true" />
        {count > 0 && <span className="font-medium tabular-nums">{count}</span>}
        {unsaved > 0 && (
          <span
            className="absolute top-0.5 right-0.5 size-2 rounded-full bg-amber-500"
            data-testid="preview-feedback-unsaved-indicator"
          >
            <span className="sr-only">{t("task:previewUnsavedDrafts", { count: unsaved })}</span>
          </span>
        )}
      </Button>
      <span className="sr-only" role="status" aria-live="polite">
        {capture.candidateLabel
          ? t("task:previewCandidate", { candidate: capture.candidateLabel })
          : ""}
      </span>
      <span
        className="sr-only"
        role="status"
        aria-live="polite"
        data-testid="preview-feedback-saved-status"
      >
        {capture.saveCount > 0 && (
          <span key={capture.saveCount}>{t("task:previewCommentSaved", { count })}</span>
        )}
      </span>
    </>
  );
}
