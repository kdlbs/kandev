"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  MAX_FILES,
  MAX_FILE_SIZE,
  MAX_TOTAL_SIZE,
  processFile,
  type FileAttachment,
} from "@/components/task/chat/file-attachment";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "@/components/task/chat/use-attachment-file-feedback";
import type { ImagePasteIssue } from "@/components/task/chat/clipboard-attachments";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { ApiError } from "@/lib/api/client";
import { restoreAttachmentPreview } from "@/lib/local-storage";
import type { StoredFileAttachment } from "@/lib/stored-chat-draft-attachments";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { t } from "@/lib/i18n";

const NANOSECONDS_PER_MILLISECOND = BigInt(1_000_000);

function attachmentExpired(expiresAt: string | undefined): boolean {
  if (!expiresAt) return false;
  const parsed = parseTurnTimestamp(expiresAt);
  return parsed === null || parsed <= BigInt(Date.now()) * NANOSECONDS_PER_MILLISECOND;
}

/** Adds processed attachments while preserving the shared count/size limits. */
export function acceptAttachmentsWithinLimits(
  current: FileAttachment[],
  incoming: FileAttachment[],
): { accepted: FileAttachment[]; rejection?: "count" | "total-size" } {
  const accepted: FileAttachment[] = [];
  let count = current.length;
  let totalSize = current.reduce((sum, attachment) => sum + attachment.size, 0);
  let rejection: "count" | "total-size" | undefined;
  for (const attachment of incoming) {
    if (count >= MAX_FILES) {
      rejection = "count";
      continue;
    }
    if (totalSize + attachment.size > MAX_TOTAL_SIZE) {
      rejection = "total-size";
      continue;
    }
    accepted.push(attachment);
    count += 1;
    totalSize += attachment.size;
  }
  return { accepted, rejection };
}

/**
 * Persists only ready staged descriptors. Inline bytes and in-flight Files
 * never enter browser storage, even for small images that carry a data URL.
 */
export function toStoredCreationAttachments(attachments: FileAttachment[]): StoredFileAttachment[] {
  return attachments.flatMap((attachment) => {
    if (!attachment.attachmentId) return [];
    return [
      {
        id: attachment.id,
        attachmentId: attachment.attachmentId,
        ...(attachment.expiresAt ? { expiresAt: attachment.expiresAt } : {}),
        mimeType: attachment.mimeType,
        fileName: attachment.fileName,
        size: attachment.size,
        isImage: attachment.isImage,
        deliveryMode: attachment.deliveryMode,
      },
    ];
  });
}

export type TaskCreateAttachmentController = {
  attachments: FileAttachment[];
  isDragging: boolean;
  isProcessing: boolean;
  isAccepting: boolean;
  setIsDragging: (dragging: boolean) => void;
  addFiles: (files: File[], issue?: ImagePasteIssue) => Promise<void>;
  handleRemoveAttachment: (id: string) => void;
  handleRetryAttachment: (id: string) => void;
  clearAttachments: (options?: { deleteStaged?: boolean }) => void;
  setAccepting: (accepting: boolean) => void;
};

const UPLOAD_FAILED_KEY = "task:uploadFailed";

type Options = {
  workspaceId: string | null | undefined;
  /** False disables processing for the unused internal controller. */
  enabled?: boolean;
  /** Invalidates in-flight work when a keyed owner unmounts. */
  invalidateOnUnmount?: boolean;
  initialAttachments?: FileAttachment[];
  onPendingAttachmentUploadsChange?: (pending: boolean) => void;
  onAttachmentsChange?: (attachments: FileAttachment[]) => boolean | void;
};

/** Keeps a creation composer's file uploads independent of dialog content. */
// eslint-disable-next-line max-lines-per-function -- one hook owns the upload lifecycle so the controller stays single-owner.
export function useTaskCreateAttachments({
  workspaceId,
  enabled = true,
  invalidateOnUnmount = false,
  initialAttachments = [],
  onPendingAttachmentUploadsChange,
  onAttachmentsChange,
}: Options): TaskCreateAttachmentController {
  const [attachments, setAttachments] = useState<FileAttachment[]>(
    enabled ? initialAttachments : [],
  );
  const attachmentsRef = useRef<FileAttachment[]>(enabled ? initialAttachments : []);
  const [processingCount, setProcessingCount] = useState(0);
  const generationRef = useRef(0);
  const acceptingRef = useRef(true);
  const [isAccepting, setIsAccepting] = useState(true);
  const initialNotifiedRef = useRef(false);

  const publishAttachments = useCallback(
    (next: FileAttachment[]): boolean => {
      if (!enabled) return true;
      return onAttachmentsChange?.(next) !== false;
    },
    [enabled, onAttachmentsChange],
  );

  const updateAttachment = useCallback(
    (id: string, update: Partial<FileAttachment>): boolean => {
      const next = attachmentsRef.current.map((attachment) =>
        attachment.id === id ? { ...attachment, ...update } : attachment,
      );
      attachmentsRef.current = next;
      setAttachments(next);
      return publishAttachments(next);
    },
    [publishAttachments],
  );

  const uploadPendingAttachment = useCallback(
    async (attachment: FileAttachment) => {
      if (!enabled || !workspaceId || !attachment.file || attachment.attachmentId) return;
      const generation = generationRef.current;
      if (!updateAttachment(attachment.id, { uploadStatus: "uploading" })) return;
      try {
        const uploaded = await uploadAttachment(attachment.file, {
          workspaceId,
          kind: attachment.isImage ? "image" : "resource",
          deliveryMode: attachment.deliveryMode,
        });
        if (
          generation !== generationRef.current ||
          !attachmentsRef.current.some((current) => current.id === attachment.id)
        ) {
          void deleteAttachment(uploaded.attachment_id).catch(() => undefined);
          return;
        }
        const keptByDraft = updateAttachment(attachment.id, {
          attachmentId: uploaded.attachment_id,
          expiresAt: uploaded.expires_at,
          uploadStatus: "ready",
          size: uploaded.size_bytes,
        });
        if (!keptByDraft) void deleteAttachment(uploaded.attachment_id).catch(() => undefined);
      } catch (error) {
        if (generation !== generationRef.current) return;
        updateAttachment(attachment.id, {
          uploadStatus: "failed",
          uploadError: error instanceof ApiError ? error.message : t(UPLOAD_FAILED_KEY),
        });
      }
    },
    [enabled, updateAttachment, workspaceId],
  );

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    for (const attachment of attachmentsRef.current) {
      if (attachment.file && !attachment.attachmentId && attachment.uploadStatus !== "uploading") {
        void uploadPendingAttachment(attachment);
      }
    }
  }, [enabled, uploadPendingAttachment, workspaceId]);

  const [isDragging, setIsDragging] = useState(false);
  const warnAttachmentCountLimit = useAttachmentCountFeedback();
  const rejectOversizedFile = useAttachmentFileFeedback();
  const warnAttachmentTotalSizeLimit = useAttachmentTotalSizeFeedback();
  const warnUnreadablePastedImage = useUnreadablePastedImageFeedback();

  useEffect(() => {
    if (!enabled) return;
    onPendingAttachmentUploadsChange?.(
      attachments.some((attachment) => attachment.file && !attachment.attachmentId),
    );
  }, [attachments, enabled, onPendingAttachmentUploadsChange, workspaceId]);

  const addFiles = useCallback(
    async (files: File[], issue?: ImagePasteIssue) => {
      if (!enabled || !acceptingRef.current) return;
      if (issue === "unreadable-image") {
        warnUnreadablePastedImage();
        return;
      }
      const generation = generationRef.current;
      setProcessingCount((count) => count + 1);
      try {
        const processed: FileAttachment[] = [];
        for (const file of files) {
          if (rejectOversizedFile(file)) continue;
          const attachment = await processFile(file);
          if (attachment) processed.push(attachment);
        }
        if (generation !== generationRef.current) return;
        if (processed.length === 0) return;

        const { accepted, rejection } = acceptAttachmentsWithinLimits(
          attachmentsRef.current,
          processed,
        );
        if (rejection === "count") {
          warnAttachmentCountLimit();
        } else if (rejection === "total-size") {
          warnAttachmentTotalSizeLimit();
        }
        if (accepted.length === 0) return;

        const next = [...attachmentsRef.current, ...accepted];
        attachmentsRef.current = next;
        setAttachments(next);
        publishAttachments(next);
        for (const attachment of accepted) void uploadPendingAttachment(attachment);
      } finally {
        if (generation === generationRef.current) {
          setProcessingCount((count) => Math.max(0, count - 1));
        }
      }
    },
    [
      enabled,
      publishAttachments,
      rejectOversizedFile,
      warnAttachmentCountLimit,
      warnAttachmentTotalSizeLimit,
      warnUnreadablePastedImage,
      uploadPendingAttachment,
    ],
  );

  const handleRemoveAttachment = useCallback(
    (id: string) => {
      if (!acceptingRef.current) return;
      const removed = attachmentsRef.current.find((attachment) => attachment.id === id);
      const next = attachmentsRef.current.filter((att) => att.id !== id);
      attachmentsRef.current = next;
      setAttachments(next);
      publishAttachments(next);
      if (removed?.attachmentId) void deleteAttachment(removed.attachmentId).catch(() => undefined);
    },
    [publishAttachments],
  );

  const clearAttachments = useCallback(
    (options?: { deleteStaged?: boolean }) => {
      generationRef.current += 1;
      setProcessingCount(0);
      const current = attachmentsRef.current;
      attachmentsRef.current = [];
      setAttachments([]);
      publishAttachments([]);
      if (options?.deleteStaged === false) return;
      current.forEach((attachment) => {
        if (attachment.attachmentId)
          void deleteAttachment(attachment.attachmentId).catch(() => undefined);
      });
    },
    [publishAttachments],
  );

  const handleRetryAttachment = useCallback(
    (id: string) => {
      if (!acceptingRef.current) return;
      const attachment = attachmentsRef.current.find((item) => item.id === id);
      if (attachment) void uploadPendingAttachment(attachment);
    },
    [uploadPendingAttachment],
  );

  useEffect(() => {
    if (!enabled || initialNotifiedRef.current) return;
    initialNotifiedRef.current = true;
    publishAttachments(attachmentsRef.current);
  }, [enabled, publishAttachments]);

  const setAccepting = useCallback((accepting: boolean) => {
    acceptingRef.current = accepting;
    setIsAccepting(accepting);
  }, []);

  useEffect(() => {
    if (!invalidateOnUnmount) return;
    return () => {
      // A late upload result from an unmounted owner must not be adopted.
      generationRef.current += 1;
      acceptingRef.current = false;
    };
  }, [invalidateOnUnmount]);

  return {
    attachments,
    isDragging,
    isProcessing: processingCount > 0,
    isAccepting,
    setIsDragging,
    addFiles,
    handleRemoveAttachment,
    handleRetryAttachment,
    clearAttachments,
    setAccepting,
  };
}

/** A blocker the user can resolve without losing ready siblings. */
export function attachmentSubmissionBlocker(
  attachments: FileAttachment[],
  isProcessing = false,
): "uploading" | "failed" | null {
  if (isProcessing) return "uploading";
  for (const attachment of attachments) {
    if (attachment.uploadStatus === "failed") return "failed";
    if (attachmentExpired(attachment.expiresAt)) return "failed";
    if (
      attachment.uploadStatus === "pending" ||
      attachment.uploadStatus === "uploading" ||
      (!attachment.attachmentId && !attachment.data) ||
      (attachment.file && !attachment.attachmentId)
    ) {
      return "uploading";
    }
  }
  return null;
}

/** Restores only descriptor state; file bytes never enter browser storage. */
export function restoreTaskCreateAttachments(
  stored: StoredFileAttachment[] | undefined,
): FileAttachment[] {
  if (!stored?.length) return [];
  const restored: FileAttachment[] = [];
  for (const item of stored) {
    const attachment = restoreAttachmentPreview(item);
    if (attachment.attachmentId) {
      const expired = attachmentExpired(attachment.expiresAt);
      restored.push({
        ...attachment,
        uploadStatus: expired ? "failed" : "ready",
        ...(expired ? { uploadError: t(UPLOAD_FAILED_KEY) } : {}),
      });
      continue;
    }
    if (!attachment.data) {
      restored.push({
        ...attachment,
        uploadStatus: "failed",
        uploadError: t(UPLOAD_FAILED_KEY),
      });
      continue;
    }
    try {
      const binary = atob(attachment.data);
      if (binary.length !== attachment.size || binary.length > MAX_FILE_SIZE) throw new Error();
      const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
      restored.push({
        ...attachment,
        file: new File([bytes], attachment.fileName, { type: attachment.mimeType }),
        uploadStatus: "pending",
      });
    } catch {
      restored.push({
        ...attachment,
        uploadStatus: "failed",
        uploadError: t(UPLOAD_FAILED_KEY),
      });
    }
  }
  return restored;
}
