"use client";

import { useCallback, useState } from "react";
import { deleteAttachment } from "@/lib/api/domains/attachment-api";
import type { usePreviewFeedback } from "@/hooks/domains/comments/use-preview-feedback";
import type { PreviewDraftEntry, usePreviewDrafts } from "./use-preview-drafts";
import { uploadPreviewScreenshot, type PreviewCaptureError } from "./use-preview-screenshots";

export type PreviewCaptureSource = {
  kind: "browser" | "html_file";
  label: string;
  sessionId?: string;
  path?: string;
};

type Options = {
  drafts: ReturnType<typeof usePreviewDrafts>;
  workspaceId: string | null;
  source: PreviewCaptureSource;
  createFeedback: ReturnType<typeof usePreviewFeedback>["create"];
  setCaptureError: (error: PreviewCaptureError | null) => void;
};

/** Uploads a screenshot draft once and records the staged attachment on the draft. */
async function ensureUploaded(
  entry: PreviewDraftEntry,
  workspaceId: string,
  drafts: ReturnType<typeof usePreviewDrafts>,
) {
  const payload = entry.payload;
  if (payload.kind !== "screenshot") return undefined;
  if (!payload.screenshot) return null;
  if (payload.screenshot.attachmentId) return payload.screenshot.attachmentId;
  const attachmentId = await uploadPreviewScreenshot(workspaceId, payload.screenshot);
  const current = drafts.find(entry.id);
  if (current?.payload.kind !== "screenshot" || !current.payload.screenshot) {
    void deleteAttachment(attachmentId).catch(() => undefined);
    return null;
  }
  const screenshot = { ...current.payload.screenshot, attachmentId };
  drafts.update(entry.id, (draft) => ({
    ...draft,
    payload: { ...current.payload, screenshot } as PreviewDraftEntry["payload"],
  }));
  return attachmentId;
}

/** Saves the open draft: uploads its screenshot when needed, then creates the item. */
export function usePreviewDraftSave({
  drafts,
  workspaceId,
  source,
  createFeedback,
  setCaptureError,
}: Options) {
  const [isUploading, setIsUploading] = useState(false);
  const [saveCount, setSaveCount] = useState(0);
  const openDraft = drafts.openDraft;

  const saveDraft = useCallback(
    async (comment: string) => {
      const entry = openDraft;
      if (!entry || !comment.trim()) return false;
      const draft = entry.payload;
      let screenshotAttachmentId: string | undefined;
      if (draft.kind === "screenshot") {
        if (!workspaceId) {
          setCaptureError("workspace");
          return false;
        }
        setIsUploading(true);
        setCaptureError(null);
        try {
          const uploaded = await ensureUploaded(entry, workspaceId, drafts);
          if (uploaded === null) return false;
          screenshotAttachmentId = uploaded;
        } catch {
          setCaptureError("upload");
          return false;
        } finally {
          setIsUploading(false);
        }
      }
      const result = await createFeedback({
        kind: draft.kind,
        comment: comment.trim(),
        sourceKind: source.kind,
        sourceSessionId: source.sessionId,
        sourceLabel: source.label,
        sourcePath: source.path,
        pageRoute: draft.page_route,
        pageTitle: draft.page_title,
        selectedText: draft.selected_text,
        textAnchor: draft.text_anchor,
        elementSnapshot: draft.element_snapshot,
        captureRect: draft.capture_rect,
        screenshotAttachmentId,
      });
      if (!result) return false;
      drafts.complete(entry.id);
      setSaveCount((count) => count + 1);
      return true;
    },
    [createFeedback, drafts, openDraft, setCaptureError, source, workspaceId],
  );

  return { saveDraft, isUploading, saveCount };
}
