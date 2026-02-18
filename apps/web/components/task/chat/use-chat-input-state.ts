import { useRef, useCallback, useState, useEffect, useMemo } from 'react';
import {
  getChatDraftText,
  setChatDraftText,
  getChatDraftAttachments,
  setChatDraftAttachments,
  setChatDraftContent,
  restoreAttachmentPreview,
} from '@/lib/local-storage';
import {
  processImageFile,
  formatBytes,
  MAX_IMAGES,
  MAX_TOTAL_SIZE,
  type ImageAttachment,
} from './image-attachment-preview';
import type { ContextItem, ImageContextItem } from '@/lib/types/context';
import type { ContextFile } from '@/lib/state/context-files-store';
import type { DiffComment } from '@/lib/diff/types';
import type { MessageAttachment } from './chat-input-container';
import type { TipTapInputHandle } from './tiptap-input';

type UseChatInputStateProps = {
  sessionId: string | null;
  isSending: boolean;
  contextItems: ContextItem[];
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  onSubmit: (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[], inlineMentions?: ContextFile[]) => void;
};

function collectComments(pendingCommentsByFile: Record<string, DiffComment[]> | undefined): DiffComment[] {
  if (!pendingCommentsByFile) return [];
  const allComments: DiffComment[] = [];
  for (const filePath of Object.keys(pendingCommentsByFile)) allComments.push(...pendingCommentsByFile[filePath]);
  return allComments;
}

function toMessageAttachments(attachments: ImageAttachment[]): MessageAttachment[] {
  return attachments.map(att => ({ type: 'image' as const, data: att.data, mime_type: att.mimeType }));
}

function clearDraft(sessionId: string | null) {
  if (!sessionId) return;
  setChatDraftText(sessionId, '');
  setChatDraftContent(sessionId, null);
  setChatDraftAttachments(sessionId, []);
}

function useAttachments(sessionId: string | null) {
  const [attachments, setAttachments] = useState<ImageAttachment[]>(() =>
    sessionId ? getChatDraftAttachments(sessionId).map(restoreAttachmentPreview) : []
  );
  const attachmentsRef = useRef(attachments);
  const prevSessionIdRef = useRef(sessionId);

  useEffect(() => {
    attachmentsRef.current = attachments;
    if (sessionId && prevSessionIdRef.current === sessionId) setChatDraftAttachments(sessionId, attachments);
  }, [attachments, sessionId]);

  const handleImagePaste = useCallback(async (files: File[]) => {
    if (attachments.length >= MAX_IMAGES) { console.warn(`Maximum ${MAX_IMAGES} images allowed`); return; }
    const currentTotalSize = attachments.reduce((sum, att) => sum + att.size, 0);
    for (const file of files) {
      if (attachments.length >= MAX_IMAGES) break;
      if (currentTotalSize + file.size > MAX_TOTAL_SIZE) { console.warn('Total attachment size limit exceeded'); break; }
      const attachment = await processImageFile(file);
      if (attachment) setAttachments(prev => [...prev, attachment]);
    }
  }, [attachments]);

  const handleRemoveAttachment = useCallback((id: string) => {
    setAttachments(prev => prev.filter(att => att.id !== id));
  }, []);

  return { attachments, attachmentsRef, prevSessionIdRef, setAttachments, handleImagePaste, handleRemoveAttachment };
}

export function useChatInputState({
  sessionId, isSending, contextItems, pendingCommentsByFile,
  showRequestChangesTooltip, onRequestChangesTooltipDismiss, onSubmit,
}: UseChatInputStateProps) {
  const [value, setValue] = useState(() => sessionId ? getChatDraftText(sessionId) : '');
  const [historyIndex, setHistoryIndex] = useState(-1);
  const inputRef = useRef<TipTapInputHandle>(null);
  const valueRef = useRef(value);
  const pendingCommentsRef = useRef(pendingCommentsByFile);

  const { attachments, attachmentsRef, prevSessionIdRef, setAttachments, handleImagePaste, handleRemoveAttachment } = useAttachments(sessionId);

  useEffect(() => {
    if (sessionId === prevSessionIdRef.current) return;
    prevSessionIdRef.current = sessionId;
    /* eslint-disable react-hooks/set-state-in-effect -- syncing from localStorage on session switch */
    setValue(sessionId ? getChatDraftText(sessionId) : '');
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [sessionId, prevSessionIdRef]);

  useEffect(() => { valueRef.current = value; }, [value]);
  useEffect(() => { pendingCommentsRef.current = pendingCommentsByFile; }, [pendingCommentsByFile]);

  const handleChange = useCallback((newValue: string) => {
    setValue(newValue);
    if (sessionId) setChatDraftText(sessionId, newValue);
    if (historyIndex >= 0) setHistoryIndex(-1);
    if (showRequestChangesTooltip && onRequestChangesTooltipDismiss) onRequestChangesTooltipDismiss();
  }, [showRequestChangesTooltip, onRequestChangesTooltipDismiss, historyIndex, sessionId]);

  const handleSubmit = useCallback((resetHeight: () => void) => {
    if (isSending) return;
    const trimmed = valueRef.current.trim();
    const allComments = collectComments(pendingCommentsRef.current);
    const currentAttachments = attachmentsRef.current;
    if (!trimmed && allComments.length === 0 && currentAttachments.length === 0) return;
    const messageAttachments = toMessageAttachments(currentAttachments);
    const inlineMentions = inputRef.current?.getMentions() ?? [];
    onSubmit(
      trimmed,
      allComments.length > 0 ? allComments : undefined,
      messageAttachments.length > 0 ? messageAttachments : undefined,
      inlineMentions.length > 0 ? inlineMentions : undefined,
    );
    inputRef.current?.clear();
    setValue('');
    setAttachments([]);
    setHistoryIndex(-1);
    resetHeight();
    clearDraft(sessionId);
  }, [onSubmit, isSending, sessionId, attachmentsRef, setAttachments]);

  const allItems = useMemo((): ContextItem[] => {
    const imageItems: ImageContextItem[] = attachments.map(att => ({
      kind: 'image' as const,
      id: `image:${att.id}`,
      label: `Image (${formatBytes(att.size)})`,
      attachment: att,
      onRemove: () => handleRemoveAttachment(att.id),
    }));
    return [...contextItems, ...imageItems];
  }, [contextItems, attachments, handleRemoveAttachment]);

  return { value, attachments, inputRef, handleImagePaste, handleChange, handleSubmit, allItems };
}
