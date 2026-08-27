"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconGitBranch, IconPaperclip } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { formatBytes } from "@/lib/utils/format-bytes";
import {
  processFile,
  MAX_FILES,
  MAX_TOTAL_SIZE,
  type FileAttachment,
} from "./chat/file-attachment";
import { readClipboardAttachments } from "./chat/clipboard-attachments";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "./chat/use-attachment-file-feedback";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { ApiError } from "@/lib/api/client";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

export function EnvironmentBadges({
  executorLabel,
  worktreeBranch,
  description,
}: {
  executorLabel: string | null;
  worktreeBranch: string | null;
  description?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {executorLabel && (
        <Badge variant="secondary" className="text-xs font-normal">
          {executorLabel}
        </Badge>
      )}
      {worktreeBranch && (
        <Badge variant="outline" className="min-w-0 max-w-full gap-1 text-xs font-normal">
          <IconGitBranch className="h-3 w-3" />
          <span className="min-w-0 truncate">{worktreeBranch}</span>
        </Badge>
      )}
      <span className="min-w-0 break-words">
        {description ?? t("task:sameEnvironmentAsCurrentSession")}
      </span>
    </div>
  );
}

export type SessionOption = { id: string; label: string; index?: number; agentName?: string };

type AttachmentLimitRejection = "count" | "total-size" | null;

function appendAttachmentsWithinLimits(
  current: FileAttachment[],
  processed: FileAttachment[],
): { attachments: FileAttachment[]; rejection: AttachmentLimitRejection } {
  let count = current.length;
  let totalSize = current.reduce((sum, attachment) => sum + attachment.size, 0);
  const accepted: FileAttachment[] = [];
  for (const attachment of processed) {
    if (count >= MAX_FILES) return { attachments: [...current, ...accepted], rejection: "count" };
    if (totalSize + attachment.size > MAX_TOTAL_SIZE) {
      return { attachments: [...current, ...accepted], rejection: "total-size" };
    }
    accepted.push(attachment);
    count += 1;
    totalSize += attachment.size;
  }
  return {
    attachments: accepted.length > 0 ? [...current, ...accepted] : current,
    rejection: null,
  };
}

function useDialogFileInput(addFiles: (files: File[]) => Promise<void>) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const handleAttachClick = useCallback(() => fileInputRef.current?.click(), []);
  const handleFileInputChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = event.target.files;
      if (files && files.length > 0) void addFiles(Array.from(files));
      event.target.value = "";
    },
    [addFiles],
  );
  return { fileInputRef, handleAttachClick, handleFileInputChange };
}

/** Unified context selector: Blank, Copy prompt, and per-session summarize options. */
export function ContextSelect({
  value,
  onValueChange,
  hasInitialPrompt,
  sessionOptions,
  isSummarizing,
}: {
  value: string;
  onValueChange: (v: string) => void;
  hasInitialPrompt: boolean;
  sessionOptions: SessionOption[];
  isSummarizing: boolean;
}) {
  const { t } = useTranslation();
  const displayLabel = useMemo(() => {
    if (value === "blank") return t("task:blank");
    if (value === "copy_prompt") return t("task:copyInitialPrompt");
    if (value.startsWith("summarize:")) {
      const sid = value.slice("summarize:".length);
      const opt = sessionOptions.find((o) => o.id === sid);
      return opt ? t("task:summarizeSessionNamed", { label: opt.label }) : t("task:summarize");
    }
    return t("task:blank");
  }, [value, sessionOptions]);

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">{t("task:context")}</label>
      <div className="flex min-w-0 items-center gap-2">
        <Select value={value} onValueChange={onValueChange} disabled={isSummarizing}>
          <SelectTrigger className="!h-11 w-full min-w-0 text-xs sm:!h-7">
            <SelectValue>{isSummarizing ? t("task:summarizing") : displayLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="blank" className="text-xs cursor-pointer">
              {t("task:blank")}
            </SelectItem>
            <SelectItem
              value="copy_prompt"
              disabled={!hasInitialPrompt}
              className="text-xs cursor-pointer"
            >
              {t("task:copyInitialPrompt")}
            </SelectItem>
            {sessionOptions.length > 0 && (
              <SelectGroup>
                <SelectLabel className="text-[11px] text-muted-foreground/70">
                  {t("task:summarizeSession")}
                </SelectLabel>
                {sessionOptions.map((opt) => (
                  <SelectItem
                    key={opt.id}
                    value={`summarize:${opt.id}`}
                    className="text-xs cursor-pointer"
                  >
                    <span className="inline-flex items-center gap-1.5">
                      {opt.index != null && (
                        <span className="text-[10px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1 py-0.5">
                          {opt.index}
                        </span>
                      )}
                      {opt.agentName && (
                        <AgentLogo agentName={opt.agentName} size={14} className="shrink-0" />
                      )}
                      {opt.label}
                    </span>
                  </SelectItem>
                ))}
              </SelectGroup>
            )}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function
export function useDialogAttachments(disabled: boolean, workspaceId?: string | null) {
  const [attachments, setAttachments] = useState<FileAttachment[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const warnAttachmentCountLimit = useAttachmentCountFeedback();
  const rejectOversizedFile = useAttachmentFileFeedback();
  const warnAttachmentTotalSizeLimit = useAttachmentTotalSizeFeedback();
  const warnUnreadablePastedImage = useUnreadablePastedImageFeedback();
  const attachmentsRef = useRef<FileAttachment[]>([]);

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
      updateAttachment(attachment.id, { uploadStatus: "uploading" });
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
          size: uploaded.size_bytes,
        });
      } catch (error) {
        updateAttachment(attachment.id, {
          uploadStatus: "failed",
          uploadError: error instanceof ApiError ? error.message : t("task:uploadFailed"),
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
    async (files: File[]) => {
      const processed: FileAttachment[] = [];
      for (const file of files) {
        if (rejectOversizedFile(file)) continue;
        const attachment = await processFile(file);
        if (attachment) processed.push(attachment);
      }
      if (processed.length === 0) return;
      const { attachments: next, rejection } = appendAttachmentsWithinLimits(
        attachmentsRef.current,
        processed,
      );
      const added = next.slice(attachmentsRef.current.length);
      attachmentsRef.current = next;
      setAttachments(next);
      for (const attachment of added) {
        void uploadPendingAttachment(attachment);
      }
      if (rejection === "count") warnAttachmentCountLimit();
      if (rejection === "total-size") warnAttachmentTotalSizeLimit();
    },
    [
      rejectOversizedFile,
      warnAttachmentCountLimit,
      warnAttachmentTotalSizeLimit,
      uploadPendingAttachment,
    ],
  );
  const { fileInputRef, handleAttachClick, handleFileInputChange } = useDialogFileInput(addFiles);

  const handleRemoveAttachment = useCallback((id: string) => {
    const removed = attachmentsRef.current.find((attachment) => attachment.id === id);
    const next = attachmentsRef.current.filter((attachment) => attachment.id !== id);
    attachmentsRef.current = next;
    setAttachments(next);
    if (removed?.attachmentId) void deleteAttachment(removed.attachmentId).catch(() => undefined);
  }, []);

  const handleRetryAttachment = useCallback(
    (id: string) => {
      const attachment = attachmentsRef.current.find((item) => item.id === id);
      if (attachment) void uploadPendingAttachment(attachment);
    },
    [uploadPendingAttachment],
  );

  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      if (disabled) return;
      const { files, issue } = readClipboardAttachments(e.clipboardData);
      if (files.length > 0) {
        e.preventDefault();
        void addFiles(files);
      } else if (issue === "unreadable-image") {
        e.preventDefault();
        warnUnreadablePastedImage();
      }
    },
    [disabled, addFiles, warnUnreadablePastedImage],
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      if (disabled) return;
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(true);
    },
    [disabled],
  );

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const rect = e.currentTarget.getBoundingClientRect();
    const { clientX, clientY } = e;
    if (
      clientX <= rect.left ||
      clientX >= rect.right ||
      clientY <= rect.top ||
      clientY >= rect.bottom
    ) {
      setIsDragging(false);
    }
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);
      if (disabled) return;
      const files = Array.from(e.dataTransfer.files).filter((f) => f.size > 0 || f.type !== "");
      if (files.length > 0) void addFiles(files);
    },
    [disabled, addFiles],
  );

  return {
    attachments,
    isDragging,
    fileInputRef,
    handleRemoveAttachment,
    handleRetryAttachment,
    handlePaste,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleAttachClick,
    handleFileInputChange,
  };
}

export function AttachButton({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center px-1 pb-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("task:attachFiles")}
            className={`h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted/40 hover:text-foreground ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
            onClick={onClick}
            disabled={disabled}
          >
            <IconPaperclip className="h-4 w-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent>{t("task:attachFiles")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function toContextItems(
  attachments: FileAttachment[],
  onRemove: (id: string) => void,
  onRetry?: (id: string) => void,
): ContextItem[] {
  return attachments.map((att) =>
    att.isImage
      ? ({
          kind: "image" as const,
          id: `image:${att.id}`,
          label: t("task:imageWithSize", { bytes: formatBytes(att.size) }),
          attachment: att,
          onRemove: () => onRemove(att.id),
          onRetry: onRetry ? () => onRetry(att.id) : undefined,
        } as ImageContextItem)
      : ({
          kind: "file-attachment" as const,
          id: `file:${att.id}`,
          label: att.fileName,
          attachment: att,
          onRemove: () => onRemove(att.id),
          onRetry: onRetry ? () => onRetry(att.id) : undefined,
        } as FileAttachmentContextItem),
  );
}
