import { useRef, useCallback, useState, useEffect, useLayoutEffect, useMemo } from "react";
import {
  getChatDraftText,
  setChatDraftText,
  getChatDraftAttachments,
  setChatDraftAttachments,
  setChatDraftContent,
  restoreAttachmentPreview,
} from "@/lib/local-storage";
import {
  processFile,
  formatBytes,
  MAX_FILES,
  MAX_TOTAL_SIZE,
  type FileAttachment,
} from "./file-attachment";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { DiffComment } from "@/lib/diff/types";
import type { MessageAttachment } from "./chat-input-container";
import type { TipTapInputHandle } from "./tiptap-input";

type UseChatInputStateProps = {
  sessionId: string | null;
  isSending: boolean;
  contextItems: ContextItem[];
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  /** Whether there are plan comments or PR feedback that allow empty-text submit */
  hasContextComments?: boolean;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  onSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => void;
};

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
  return attachments.map((att) =>
    att.isImage
      ? { type: "image" as const, data: att.data, mime_type: att.mimeType }
      : { type: "resource" as const, data: att.data, mime_type: att.mimeType, name: att.fileName },
  );
}

function clearDraft(sessionId: string | null) {
  if (!sessionId) return;
  setChatDraftText(sessionId, "");
  setChatDraftContent(sessionId, null);
  setChatDraftAttachments(sessionId, []);
}

function useAttachments(sessionId: string | null) {
  const [attachments, setAttachments] = useState<FileAttachment[]>(() =>
    sessionId ? getChatDraftAttachments(sessionId).map(restoreAttachmentPreview) : [],
  );
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

  const addFiles = useCallback(
    async (files: File[]) => {
      if (attachments.length >= MAX_FILES) {
        console.warn(`Maximum ${MAX_FILES} files allowed`);
        return;
      }
      const currentTotalSize = attachments.reduce((sum, att) => sum + att.size, 0);
      for (const file of files) {
        if (attachments.length >= MAX_FILES) break;
        if (currentTotalSize + file.size > MAX_TOTAL_SIZE) {
          console.warn("Total attachment size limit exceeded");
          break;
        }
        const attachment = await processFile(file);
        if (attachment) setAttachments((prev) => [...prev, attachment]);
      }
    },
    [attachments],
  );

  const handleRemoveAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((att) => att.id !== id));
  }, []);

  const getAttachments = useCallback(
    () => toMessageAttachments(attachmentsRef.current),
    [attachmentsRef],
  );

  return {
    attachments,
    attachmentsRef,
    setAttachments,
    addFiles,
    handleRemoveAttachment,
    getAttachments,
  };
}

export function useChatInputState({
  sessionId,
  isSending,
  contextItems,
  pendingCommentsByFile,
  hasContextComments = false,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  onSubmit,
}: UseChatInputStateProps) {
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
    getAttachments,
  } = useAttachments(sessionId);

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
      if (isSending) return;
      const trimmed = valueRef.current.trim();
      const allComments = collectComments(pendingCommentsRef.current);
      const currentAttachments = attachmentsRef.current;
      const hasContent =
        trimmed || allComments.length > 0 || currentAttachments.length > 0 || hasContextComments;
      if (!hasContent) return;
      const messageAttachments = toMessageAttachments(currentAttachments);
      const inlineMentions = inputRef.current?.getMentions() ?? [];
      onSubmit(
        trimmed,
        allComments.length > 0 ? allComments : undefined,
        messageAttachments.length > 0 ? messageAttachments : undefined,
        inlineMentions.length > 0 ? inlineMentions : undefined,
      );
      inputRef.current?.clear();
      setValue("");
      setAttachments([]);
      setHistoryIndex(-1);
      resetHeight();
      clearDraft(sessionId);
    },
    [onSubmit, isSending, sessionId, attachmentsRef, setAttachments, hasContextComments],
  );

  const allItems = useMemo((): ContextItem[] => {
    const attachmentItems: (ImageContextItem | FileAttachmentContextItem)[] = attachments.map(
      (att) =>
        att.isImage
          ? ({
              kind: "image" as const,
              id: `image:${att.id}`,
              label: `Image (${formatBytes(att.size)})`,
              attachment: att,
              onRemove: () => handleRemoveAttachment(att.id),
            } as ImageContextItem)
          : ({
              kind: "file-attachment" as const,
              id: `file:${att.id}`,
              label: att.fileName,
              attachment: att,
              onRemove: () => handleRemoveAttachment(att.id),
            } as FileAttachmentContextItem),
    );
    return [...contextItems, ...attachmentItems];
  }, [contextItems, attachments, handleRemoveAttachment]);

  // prettier-ignore
  return { value, attachments, inputRef, addFiles, handleChange, handleSubmit, allItems, getAttachments };
}
