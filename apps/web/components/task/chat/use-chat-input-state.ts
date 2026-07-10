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
import type { ChatSubmitResult, MessageAttachment } from "./chat-input-container";
import type { TipTapInputHandle } from "./tiptap-input";
import type { TaskMentionData } from "@/hooks/use-inline-mention";

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
    inlineTaskMentions?: TaskMentionData[],
  ) => ChatSubmitResult;
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
  return attachments.map((att) =>
    att.isImage
      ? {
          type: "image" as const,
          data: att.data,
          mime_type: att.mimeType,
          name: att.fileName,
          ...(att.deliveryMode === "path" && { delivery_mode: "path" as const }),
        }
      : {
          type: "resource" as const,
          data: att.data,
          mime_type: att.mimeType,
          name: att.fileName,
          delivery_mode: "path" as const,
        },
  );
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
  valueRef: MutableRefObject<string>;
  pendingCommentsRef: MutableRefObject<Record<string, DiffComment[]> | undefined>;
  attachmentsRef: MutableRefObject<FileAttachment[]>;
  hasContextComments: boolean;
  inputRef: RefObject<TipTapInputHandle | null>;
  onSubmit: UseChatInputStateProps["onSubmit"];
  clearArgs: Omit<ClearSubmittedInputArgs, "submittedText" | "submittedAttachments">;
};

function submitDraft(args: SubmitDraftArgs) {
  if (args.isSending) return;
  const trimmed = args.valueRef.current.trim();
  const allComments = collectComments(args.pendingCommentsRef.current);
  const currentAttachments = args.attachmentsRef.current;
  const submittedAttachments = attachmentSnapshot(currentAttachments);
  const hasContent =
    trimmed || allComments.length > 0 || currentAttachments.length > 0 || args.hasContextComments;
  if (!hasContent) return;
  const messageAttachments = toMessageAttachments(currentAttachments);
  const inlineMentions = args.inputRef.current?.getMentions() ?? [];
  const inlineTaskMentions = args.inputRef.current?.getTaskMentions() ?? [];
  const result = args.onSubmit(
    trimmed,
    allComments.length > 0 ? allComments : undefined,
    messageAttachments.length > 0 ? messageAttachments : undefined,
    inlineMentions.length > 0 ? inlineMentions : undefined,
    inlineTaskMentions.length > 0 ? inlineTaskMentions : undefined,
  );
  handleSubmitResult(result, () =>
    clearSubmittedInput({
      ...args.clearArgs,
      submittedText: trimmed,
      submittedAttachments,
    }),
  );
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
    setAttachments((prev) => {
      const next = prev.filter((att) => att.id !== id);
      attachmentsRef.current = next;
      return next;
    });
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

  return {
    attachments,
    attachmentsRef,
    setAttachments,
    addFiles,
    handleRemoveAttachment,
    handleDeliveryModeChange,
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
    handleDeliveryModeChange,
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
      submitDraft({
        isSending,
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
              onDeliveryModeChange: (mode) => handleDeliveryModeChange(att.id, mode),
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
  }, [contextItems, attachments, handleRemoveAttachment, handleDeliveryModeChange]);

  // prettier-ignore
  return { value, attachments, inputRef, addFiles, handleChange, handleSubmit, allItems, getAttachments };
}
