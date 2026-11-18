"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { SelectionCommentSurface } from "@/components/shared/selection-comment/selection-comment-surface";
import { EditorSelectionCommentTrigger } from "@/components/editors/editor-selection-comment-trigger";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import { PendingFeedbackEditor } from "./preview-feedback-collection";
import {
  PreviewFeedbackEvidence,
  evidenceForDraft,
  evidenceText,
} from "./preview-feedback-evidence";

const SELECTION_BUTTON_WIDTH = 110;

function useDraftErrorMessage(capture: PreviewCaptureController) {
  const { t } = useTranslation();
  if (capture.captureError) return t("task:previewScreenshotFailed");
  if (capture.mutationError) return t("task:previewMutationFailed");
  return null;
}

function fallbackAnchor(host: RefObject<HTMLElement | null>) {
  const bounds = host.current?.getBoundingClientRect();
  return bounds ? { x: bounds.left + 16, y: bounds.top + 16 } : { x: 16, y: 16 };
}

/** The shared comment surface for the open unsaved draft. */
export function PreviewDraftSurface({
  capture,
  host,
}: {
  capture: PreviewCaptureController;
  host: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  const errorMessage = useDraftErrorMessage(capture);
  const entry = capture.openDraft;
  if (!entry) return null;
  const draft = entry.payload;
  const evidence = evidenceForDraft(
    draft,
    draft.kind === "screenshot"
      ? t("task:previewScreenshotDimensions", {
          width: draft.screenshot.width,
          height: draft.screenshot.height,
        })
      : "",
  );
  return (
    <SelectionCommentSurface
      key={entry.id}
      selectedText={evidenceText(evidence) || draft.page_route}
      contextLabel={draft.page_route}
      evidence={
        <>
          <PreviewFeedbackEvidence evidence={evidence} />
          {capture.isUploading && (
            <p role="status" className="mt-1 text-xs text-muted-foreground">
              {t("task:previewUploadingScreenshot")}
            </p>
          )}
        </>
      }
      position={capture.draftAnchor ?? fallbackAnchor(host)}
      draftComment={entry.comment}
      onDraftChange={capture.setDraftComment}
      inputLabel={t("task:previewCommentSelection")}
      errorMessage={errorMessage}
      testId="preview-feedback-draft"
      inputTestId="preview-feedback-draft-input"
      addButtonTestId="preview-feedback-draft-save"
      onAdd={(comment) => capture.saveDraft(comment)}
      onClose={capture.closeDraftSurface}
    />
  );
}

/** Edits the comment of a saved region opened from its outline. */
export function PreviewSavedRegionEditor({
  capture,
  host,
}: {
  capture: PreviewCaptureController;
  host: RefObject<HTMLElement | null>;
}) {
  const item = capture.editingItem;
  if (!item) return null;
  return (
    <PendingFeedbackEditor
      key={item.id}
      item={item}
      collection={capture}
      position={capture.editingAnchor ?? fallbackAnchor(host)}
      onClose={capture.closeItemEditor}
    />
  );
}

/** Comment button beside a page selection on fine pointers; the bottom dock on touch. */
export function PreviewSelectionComment({
  capture,
  touch,
  host,
}: {
  capture: PreviewCaptureController;
  touch: boolean;
  host: RefObject<HTMLElement | null>;
}) {
  const rect = capture.selectionRect;
  if (!rect || capture.openDraft) return null;
  const width = host.current?.clientWidth ?? Number.POSITIVE_INFINITY;
  const left = Math.max(0, Math.min(rect.x + rect.width - 16, width - SELECTION_BUTTON_WIDTH));
  return (
    <EditorSelectionCommentTrigger
      touch={touch}
      position={{ left, top: rect.y + rect.height + 6 }}
      positioning="absolute"
      hostRef={host}
      className="pointer-events-auto"
      testId="preview-selection-comment"
      onOpen={capture.captureSelection}
    />
  );
}
