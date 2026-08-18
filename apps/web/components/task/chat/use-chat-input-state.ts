import {
  useRef,
  useCallback,
  useState,
  useEffect,
  useLayoutEffect,
  useMemo,
  type Dispatch,
  type MutableRefObject,
  type RefObject,
  type SetStateAction,
} from "react";
import {
  getChatDraftText,
  setChatDraftText,
  getChatDraftAttachments,
  setChatDraftAttachments,
  setChatDraftContent,
  restoreAttachmentPreview,
} from "@/lib/local-storage";
import { formatBytes } from "@/lib/utils/format-bytes";
import { processFile, MAX_FILES, MAX_TOTAL_SIZE, type FileAttachment } from "./file-attachment";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "./use-attachment-file-feedback";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";
import type { DiffComment } from "@/lib/diff/types";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
  MessageAttachment,
} from "./chat-input-container";
import type { TipTapInputHandle } from "./tiptap-input";
import type { ImagePasteIssue } from "./clipboard-attachments";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { ApiError } from "@/lib/api/client";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type UseChatInputStateProps = {
  sessionId: string | null;
  workspaceId?: string | null;
  isSending: boolean;
  contextItems: ContextItem[];
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  /** Whether there are plan comments or PR feedback that allow empty-text submit */
  hasContextComments?: boolean;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  onSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
};

function isPromiseLike(value: ChatSubmitResult): value is Promise<void | boolean> {
  return (
    typeof value === "object" &&
    value !== null &&
    "then" in value &&
    typeof value.then === "function"
  );
}

function collectComments(
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined,
): DiffComment[] {
  if (!pendingCommentsByFile) return [];
  const allComments: DiffComment[] = [];
  for (const filePath of Object.keys(pendingCommentsByFile))
    allComments.push(...pendingCommentsByFile[filePath]);
  return allComments;
}

function toMessageAttachments(attachments: FileAttachment[]): MessageAttachment[] {
  return attachments.map((att) => {
    const attachment = {
      type: att.isImage ? ("image" as const) : ("resource" as const),
      mime_type: att.mimeType,
      name: att.fileName,
      size_bytes: att.size,
      ...(att.attachmentId ? { attachment_id: att.attachmentId } : { data: att.data ?? "" }),
      ...(att.deliveryMode === "path" && { delivery_mode: "path" as const }),
    };
    return attachment;
  });
}

function clearDraft(sessionId: string | null) {
  if (!sessionId) return;
  setChatDraftText(sessionId, "");
  setChatDraftContent(sessionId, null);
  setChatDraftAttachments(sessionId, []);
}

function clearDraftText(sessionId: string | null) {
  if (!sessionId) return;
  setChatDraftText(sessionId, "");
  setChatDraftContent(sessionId, null);
}

function attachmentSnapshot(attachments: FileAttachment[]): string {
  return attachments.map((att) => `${att.id}:${att.deliveryMode ?? "prompt"}`).join("|");
}

type ClearSubmittedInputArgs = {
  valueRef: MutableRefObject<string>;
  submittedText: string;
  attachmentsRef: MutableRefObject<FileAttachment[]>;
  submittedAttachments: string;
  inputRef: RefObject<TipTapInputHandle | null>;
  setValue: Dispatch<SetStateAction<string>>;
  setAttachments: Dispatch<SetStateAction<FileAttachment[]>>;
  setHistoryIndex: Dispatch<SetStateAction<number>>;
  resetHeight: () => void;
  sessionId: string | null;
};

function clearSubmittedInput(args: ClearSubmittedInputArgs) {
  // Abort if the user already typed new content since this submit started.
  if (args.valueRef.current.trim() !== args.submittedText) return;
  const attachmentsChanged =
    attachmentSnapshot(args.attachmentsRef.current) !== args.submittedAttachments;
  args.inputRef.current?.clear();
  args.setValue("");
  args.setHistoryIndex(-1);
  args.resetHeight();
  if (attachmentsChanged) {
    clearDraftText(args.sessionId);
    return;
  }
  args.setAttachments([]);
  clearDraft(args.sessionId);
}

function handleSubmitResult(result: ChatSubmitResult, onSuccess: () => void) {
  if (isPromiseLike(result)) {
    // Submitters should show user-visible failure feedback and resolve false;
    // rejected promises are unexpected, so preserve the draft and log them.
    void result
      .then((submitted) => {
        if (submitted !== false) onSuccess();
      })
      .catch((error) => {
        console.error("Failed to submit chat input:", error);
      });
    return;
  }
  if (result !== false) onSuccess();
}

type SubmitDraftArgs = {
  isSending: boolean;
  workspaceId?: string | null;
  valueRef: MutableRefObject<string>;
  pendingCommentsRef: MutableRefObject<Record<string, DiffComment[]> | undefined>;
  attachmentsRef: MutableRefObject<FileAttachment[]>;
  hasContextComments: boolean;
  inputRef: RefObject<TipTapInputHandle | null>;
  onSubmit: UseChatInputStateProps["onSubmit"];
  clearArgs: Omit<ClearSubmittedInputArgs, "submittedText" | "submittedAttachments">;
};

function buildChatSubmitPayload(payload: Required<ChatSubmitPayload>): ChatSubmitPayload {
  return Object.fromEntries(
    Object.entries(payload).filter(
      ([key, value]) => key === "message" || (Array.isArray(value) && value.length > 0),
    ),
  ) as ChatSubmitPayload;
}

function submitDraft(args: SubmitDraftArgs) {
  if (args.isSending) return;
  const trimmed = args.valueRef.current.trim();
  const allComments = collectComments(args.pendingCommentsRef.current);
  const currentAttachments = args.attachmentsRef.current;
  if (
    args.workspaceId &&
    currentAttachments.some((attachment) => attachment.file && !attachment.attachmentId)
  ) {
    return;
  }
  const submittedAttachments = attachmentSnapshot(currentAttachments);
  const hasContent =
    trimmed || allComments.length > 0 || currentAttachments.length > 0 || args.hasContextComments;
  if (!hasContent) return;
  const messageAttachments = toMessageAttachments(currentAttachments);
  const inlineMentions = args.inputRef.current?.getMentions() ?? [];
  const inlineTaskMentions = args.inputRef.current?.getTaskMentions() ?? [];
  const entityReferences = args.inputRef.current?.getEntityReferences() ?? [];
  const result = args.onSubmit(
    buildChatSubmitPayload({
      message: trimmed,
      reviewComments: allComments,
      attachments: messageAttachments,
      inlineMentions,
      inlineTaskMentions,
      entityReferences,
    }),
  );
  handleSubmitResult(result, () =>
    clearSubmittedInput({
      ...args.clearArgs,
      submittedText: trimmed,
      submittedAttachments,
    }),
  );
}

// Attachment staging, retry, and draft persistence share one state machine so
// desktop and mobile composers cannot drift.
// eslint-disable-next-line max-lines-per-function
function useAttachments(sessionId: string | null, workspaceId?: string | null) {
  const [attachments, setAttachments] = useState<FileAttachment[]>(() =>
    sessionId ? getChatDraftAttachments(sessionId).map(restoreAttachmentPreview) : [],
  );
  const warnAttachmentCountLimit = useAttachmentCountFeedback();
  const rejectOversizedFile = useAttachmentFileFeedback();
  const warnAttachmentTotalSizeLimit = useAttachmentTotalSizeFeedback();
  const warnUnreadablePastedImage = useUnreadablePastedImageFeedback();
  const attachmentsRef = useRef(attachments);
  const prevSessionIdRef = useRef(sessionId);
  const prevPersistSessionIdRef = useRef(sessionId);

  // Reset attachments from storage when session changes (runs before paint)
  useLayoutEffect(() => {
    if (sessionId === prevSessionIdRef.current) return;
    prevSessionIdRef.current = sessionId;
    const newAttachments = sessionId
      ? getChatDraftAttachments(sessionId).map(restoreAttachmentPreview)
      : [];
    /* eslint-disable react-hooks/set-state-in-effect -- syncing from localStorage on session switch */
    setAttachments(newAttachments);
    /* eslint-enable react-hooks/set-state-in-effect */
    attachmentsRef.current = newAttachments;
  }, [sessionId]);

  // Persist attachments to storage when they change (for the same session)
  useEffect(() => {
    // Skip first invocation after session change to avoid overwriting freshly loaded attachments
    if (sessionId !== prevPersistSessionIdRef.current) {
      prevPersistSessionIdRef.current = sessionId;
      return;
    }
    attachmentsRef.current = attachments;
    if (sessionId) setChatDraftAttachments(sessionId, attachments);
  }, [attachments, sessionId]);

  const updateAttachment = useCallback((id: string, update: Partial<FileAttachment>) => {
    setAttachments((prev) => {
      const next = prev.map((attachment) =>
        attachment.id === id ? { ...attachment, ...update } : attachment,
      );
      attachmentsRef.current = next;
      return next;
    });
  }, []);

  const uploadPendingAttachment = useCallback(
    async (attachment: FileAttachment) => {
      if (!workspaceId || !attachment.file || attachment.attachmentId) return;
      updateAttachment(attachment.id, { uploadStatus: "uploading", uploadError: undefined });
      try {
        const uploaded = await uploadAttachment(attachment.file, {
          workspaceId,
          kind: attachment.isImage ? "image" : "resource",
          deliveryMode: attachment.deliveryMode,
        });
        if (!attachmentsRef.current.some((current) => current.id === attachment.id)) {
          void deleteAttachment(uploaded.attachment_id).catch(() => undefined);
          return;
        }
        updateAttachment(attachment.id, {
          attachmentId: uploaded.attachment_id,
          uploadStatus: "ready",
          uploadError: undefined,
          size: uploaded.size_bytes,
        });
      } catch (error) {
        updateAttachment(attachment.id, {
          uploadStatus: "failed",
          uploadError: error instanceof ApiError ? error.message : t("task:attachmentUploadFailed"),
        });
      }
    },
    [updateAttachment, workspaceId],
  );

  useEffect(() => {
    if (!workspaceId) return;
    for (const attachment of attachmentsRef.current) {
      if (attachment.file && !attachment.attachmentId && attachment.uploadStatus !== "uploading") {
        void uploadPendingAttachment(attachment);
      }
    }
  }, [uploadPendingAttachment, workspaceId]);

  const addFiles = useCallback(
    async (files: File[], issue?: ImagePasteIssue) => {
      if (issue === "unreadable-image") {
        warnUnreadablePastedImage();
        return;
      }
      if (attachments.length >= MAX_FILES) {
        warnAttachmentCountLimit();
        return;
      }
      let acceptedCount = attachments.length;
      let acceptedTotalSize = attachments.reduce((sum, att) => sum + att.size, 0);
      for (const file of files) {
        if (acceptedCount >= MAX_FILES) {
          warnAttachmentCountLimit();
          break;
        }
        if (rejectOversizedFile(file)) continue;
        if (acceptedTotalSize + file.size > MAX_TOTAL_SIZE) {
          warnAttachmentTotalSizeLimit();
          break;
        }
        const attachment = await processFile(file);
        if (attachment) {
          acceptedCount += 1;
          acceptedTotalSize += attachment.size;
          const staged = {
            ...attachment,
            uploadStatus: workspaceId ? ("pending" as const) : attachment.uploadStatus,
          };
          setAttachments((prev) => {
            const next = [...prev, staged];
            attachmentsRef.current = next;
            return next;
          });
          void uploadPendingAttachment(staged);
        }
      }
    },
    [
      attachments,
      rejectOversizedFile,
      warnAttachmentCountLimit,
      warnAttachmentTotalSizeLimit,
      warnUnreadablePastedImage,
      uploadPendingAttachment,
      workspaceId,
    ],
  );

  const handleRemoveAttachment = useCallback((id: string) => {
    const removed = attachmentsRef.current.find((attachment) => attachment.id === id);
    setAttachments((prev) => {
      const next = prev.filter((att) => att.id !== id);
      attachmentsRef.current = next;
      return next;
    });
    if (removed?.attachmentId) void deleteAttachment(removed.attachmentId).catch(() => undefined);
  }, []);

  const handleDeliveryModeChange = useCallback((id: string, deliveryMode: "prompt" | "path") => {
    setAttachments((prev) => {
      const next = prev.map((att) => (att.id === id ? { ...att, deliveryMode } : att));
      attachmentsRef.current = next;
      return next;
    });
  }, []);

  const getAttachments = useCallback(
    () => toMessageAttachments(attachmentsRef.current),
    [attachmentsRef],
  );

  const handleRetryAttachment = useCallback(
    (id: string) => {
      const attachment = attachmentsRef.current.find((item) => item.id === id);
      if (attachment) void uploadPendingAttachment(attachment);
    },
    [uploadPendingAttachment],
  );

  return {
    attachments,
    attachmentsRef,
    setAttachments,
    addFiles,
    handleRemoveAttachment,
    handleDeliveryModeChange,
    handleRetryAttachment,
    getAttachments,
  };
}

// eslint-disable-next-line max-lines-per-function
export function useChatInputState({
  sessionId,
  workspaceId,
  isSending,
  contextItems,
  pendingCommentsByFile,
  hasContextComments = false,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  onSubmit,
}: UseChatInputStateProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState(() => (sessionId ? getChatDraftText(sessionId) : ""));
  const [historyIndex, setHistoryIndex] = useState(-1);
  const inputRef = useRef<TipTapInputHandle>(null);
  const valueRef = useRef(value);
  const pendingCommentsRef = useRef(pendingCommentsByFile);
  const prevTextSessionIdRef = useRef(sessionId);

  const {
    attachments,
    attachmentsRef,
    setAttachments,
    addFiles,
    handleRemoveAttachment,
    handleDeliveryModeChange,
    handleRetryAttachment,
    getAttachments,
  } = useAttachments(sessionId, workspaceId);

  // Reset text value from storage when session changes (runs before paint)
  useLayoutEffect(() => {
    if (sessionId === prevTextSessionIdRef.current) return;
    prevTextSessionIdRef.current = sessionId;
    /* eslint-disable react-hooks/set-state-in-effect -- syncing from localStorage on session switch */
    setValue(sessionId ? getChatDraftText(sessionId) : "");
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [sessionId]);

  useEffect(() => {
    valueRef.current = value;
    pendingCommentsRef.current = pendingCommentsByFile;
  }, [value, pendingCommentsByFile]);

  const handleChange = useCallback(
    (newValue: string) => {
      // TipTap renders its document before React's passive effects flush. Keep
      // the submit snapshot in sync with the editor event so a fast submit
      // cannot observe the previous draft and silently no-op.
      valueRef.current = newValue;
      setValue(newValue);
      if (sessionId) setChatDraftText(sessionId, newValue);
      if (historyIndex >= 0) setHistoryIndex(-1);
      if (showRequestChangesTooltip && onRequestChangesTooltipDismiss)
        onRequestChangesTooltipDismiss();
    },
    [showRequestChangesTooltip, onRequestChangesTooltipDismiss, historyIndex, sessionId],
  );

  const handleSubmit = useCallback(
    (resetHeight: () => void) => {
      submitDraft({
        isSending,
        workspaceId,
        valueRef,
        pendingCommentsRef,
        attachmentsRef,
        hasContextComments,
        inputRef,
        onSubmit,
        clearArgs: {
          valueRef,
          attachmentsRef,
          inputRef,
          setValue,
          setAttachments,
          setHistoryIndex,
          resetHeight,
          sessionId,
        },
      });
    },
    [
      onSubmit,
      isSending,
      workspaceId,
      sessionId,
      attachmentsRef,
      setAttachments,
      hasContextComments,
    ],
  );

  const allItems = useMemo((): ContextItem[] => {
    const attachmentItems: (ImageContextItem | FileAttachmentContextItem)[] = attachments.map(
      (att) =>
        att.isImage
          ? ({
              kind: "image" as const,
              id: `image:${att.id}`,
              label: t("task:imageWithSize", { bytes: formatBytes(att.size) }),
              attachment: att,
              onRemove: () => handleRemoveAttachment(att.id),
              onDeliveryModeChange: (mode) => handleDeliveryModeChange(att.id, mode),
              onRetry: () => handleRetryAttachment(att.id),
            } as ImageContextItem)
          : ({
              kind: "file-attachment" as const,
              id: `file:${att.id}`,
              label: att.fileName,
              attachment: att,
              onRemove: () => handleRemoveAttachment(att.id),
              onRetry: () => handleRetryAttachment(att.id),
            } as FileAttachmentContextItem),
    );
    return [...contextItems, ...attachmentItems];
  }, [
    contextItems,
    attachments,
    handleRemoveAttachment,
    handleDeliveryModeChange,
    handleRetryAttachment,
  ]);

  const hasPendingAttachmentUploads =
    Boolean(workspaceId) &&
    attachments.some((attachment) => attachment.file && !attachment.attachmentId);

  // prettier-ignore
  return { value, attachments, inputRef, addFiles, handleChange, handleSubmit, allItems, getAttachments, hasPendingAttachmentUploads };
}
