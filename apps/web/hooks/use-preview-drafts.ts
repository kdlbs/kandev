"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { deleteAttachment } from "@/lib/api/domains/attachment-api";
import type { PreviewCaptureDraft, PreviewScreenshotRegion } from "@/lib/preview-inspect-bridge";
import { generateUUID } from "@/lib/utils";

export type PreviewScreenshotDraft = PreviewScreenshotRegion & {
  kind: "screenshot";
  selected_text?: undefined;
  text_anchor?: undefined;
  element_snapshot?: undefined;
  /** `null` while the region is still being captured. */
  screenshot: PreviewDraftScreenshot | null;
};

export type PreviewDraftScreenshot = {
  blob: Blob;
  width: number;
  height: number;
  previewUrl: string;
  fileName: string;
  attachmentId?: string;
  /** The region covers a canvas but rasterized to one colour. */
  uniformOverCanvas?: boolean;
};

export type PreviewFeedbackDraft = PreviewCaptureDraft | PreviewScreenshotDraft;

/** One unsaved capture. At most one entry is open in a comment surface. */
export type PreviewDraftEntry = {
  id: string;
  payload: PreviewFeedbackDraft;
  comment: string;
  open: boolean;
};

function releaseImage(payload: PreviewFeedbackDraft, deleteUpload: boolean) {
  if (payload.kind !== "screenshot" || !payload.screenshot) return;
  URL.revokeObjectURL(payload.screenshot.previewUrl);
  if (deleteUpload && payload.screenshot.attachmentId) {
    void deleteAttachment(payload.screenshot.attachmentId).catch(() => undefined);
  }
}

/** Ordered unsaved captures for one preview, kept until saved, discarded, or unmounted. */
export function usePreviewDrafts() {
  const [drafts, setDrafts] = useState<PreviewDraftEntry[]>([]);
  const draftsRef = useRef<PreviewDraftEntry[]>([]);

  const commit = useCallback((next: PreviewDraftEntry[]) => {
    draftsRef.current = next;
    setDrafts(next);
  }, []);

  useEffect(
    () => () => {
      draftsRef.current.forEach((draft) => releaseImage(draft.payload, true));
      draftsRef.current = [];
    },
    [],
  );

  const add = useCallback(
    (payload: PreviewFeedbackDraft) => {
      const id = generateUUID();
      commit([
        ...draftsRef.current.map((draft) => ({ ...draft, open: false })),
        { id, payload, comment: "", open: true },
      ]);
      return id;
    },
    [commit],
  );

  const open = useCallback(
    (id: string) => commit(draftsRef.current.map((draft) => ({ ...draft, open: draft.id === id }))),
    [commit],
  );

  const close = useCallback(
    () => commit(draftsRef.current.map((draft) => ({ ...draft, open: false }))),
    [commit],
  );

  const remove = useCallback(
    (id: string, deleteUpload: boolean) => {
      const target = draftsRef.current.find((draft) => draft.id === id);
      if (!target) return;
      releaseImage(target.payload, deleteUpload);
      commit(draftsRef.current.filter((draft) => draft.id !== id));
    },
    [commit],
  );

  const update = useCallback(
    (id: string, change: (draft: PreviewDraftEntry) => PreviewDraftEntry) =>
      commit(draftsRef.current.map((draft) => (draft.id === id ? change(draft) : draft))),
    [commit],
  );

  const setComment = useCallback(
    (id: string, comment: string) => update(id, (draft) => ({ ...draft, comment })),
    [update],
  );

  const replaceScreenshot = useCallback(
    (id: string, payload: PreviewScreenshotDraft) => {
      const target = draftsRef.current.find((draft) => draft.id === id);
      if (!target) return;
      releaseImage(target.payload, true);
      update(id, (draft) => ({ ...draft, payload }));
    },
    [update],
  );

  const clear = useCallback(() => {
    draftsRef.current.forEach((draft) => releaseImage(draft.payload, true));
    commit([]);
  }, [commit]);

  return {
    drafts,
    openDraft: drafts.find((draft) => draft.open) ?? null,
    find: useCallback((id: string) => draftsRef.current.find((draft) => draft.id === id), []),
    add,
    open,
    close,
    setComment,
    update,
    replaceScreenshot,
    /** Drops a draft and its staged upload. */
    discard: useCallback((id: string) => remove(id, true), [remove]),
    /** Drops a saved draft; its upload now belongs to the saved item. */
    complete: useCallback((id: string) => remove(id, false), [remove]),
    clear,
  };
}
