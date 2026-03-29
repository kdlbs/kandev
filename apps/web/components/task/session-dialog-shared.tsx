"use client";

import { useCallback, useMemo, useRef, useState } from "react";
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
import {
  processFile,
  formatBytes,
  MAX_FILES,
  MAX_TOTAL_SIZE,
  type FileAttachment,
} from "./chat/file-attachment";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";

export function EnvironmentBadges({
  executorLabel,
  worktreeBranch,
  description,
}: {
  executorLabel: string | null;
  worktreeBranch: string | null;
  description?: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {executorLabel && (
        <Badge variant="secondary" className="text-xs font-normal">
          {executorLabel}
        </Badge>
      )}
      {worktreeBranch && (
        <Badge variant="outline" className="text-xs font-normal gap-1">
          <IconGitBranch className="h-3 w-3" />
          {worktreeBranch}
        </Badge>
      )}
      <span>{description ?? "Same environment as current session"}</span>
    </div>
  );
}

export type SessionOption = { id: string; label: string; index?: number; agentName?: string };

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
  const displayLabel = useMemo(() => {
    if (value === "blank") return "Blank";
    if (value === "copy_prompt") return "Copy initial prompt";
    if (value.startsWith("summarize:")) {
      const sid = value.slice("summarize:".length);
      const opt = sessionOptions.find((o) => o.id === sid);
      return opt ? `Summarize ${opt.label}` : "Summarize";
    }
    return "Blank";
  }, [value, sessionOptions]);

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">Context</label>
      <div className="flex items-center gap-2">
        <Select value={value} onValueChange={onValueChange} disabled={isSummarizing}>
          <SelectTrigger className="w-full text-xs">
            <SelectValue>{isSummarizing ? "Summarizing..." : displayLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="blank" className="text-xs cursor-pointer">
              Blank
            </SelectItem>
            <SelectItem
              value="copy_prompt"
              disabled={!hasInitialPrompt}
              className="text-xs cursor-pointer"
            >
              Copy initial prompt
            </SelectItem>
            {sessionOptions.length > 0 && (
              <SelectGroup>
                <SelectLabel className="text-[11px] text-muted-foreground/70">
                  Summarize session
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

export function useDialogAttachments(disabled: boolean) {
  const [attachments, setAttachments] = useState<FileAttachment[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const addFiles = useCallback(async (files: File[]) => {
    const processed: FileAttachment[] = [];
    for (const file of files) {
      const attachment = await processFile(file);
      if (attachment) processed.push(attachment);
    }
    if (processed.length === 0) return;
    setAttachments((prev) => {
      let count = prev.length;
      let totalSize = prev.reduce((s, a) => s + a.size, 0);
      const accepted: FileAttachment[] = [];
      for (const att of processed) {
        if (count >= MAX_FILES || totalSize + att.size > MAX_TOTAL_SIZE) break;
        accepted.push(att);
        count += 1;
        totalSize += att.size;
      }
      return accepted.length > 0 ? [...prev, ...accepted] : prev;
    });
  }, []);

  const handleRemoveAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      if (disabled) return;
      const files: File[] = [];
      for (const item of e.clipboardData?.items ?? []) {
        if (item.kind === "file") {
          const f = item.getAsFile();
          if (f) files.push(f);
        }
      }
      if (files.length > 0) {
        e.preventDefault();
        void addFiles(files);
      }
    },
    [disabled, addFiles],
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

  const handleAttachClick = useCallback(() => fileInputRef.current?.click(), []);

  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (files && files.length > 0) void addFiles(Array.from(files));
      e.target.value = "";
    },
    [addFiles],
  );

  return {
    attachments,
    isDragging,
    fileInputRef,
    handleRemoveAttachment,
    handlePaste,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleAttachClick,
    handleFileInputChange,
  };
}

export function AttachButton({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
  return (
    <div className="flex items-center px-1 pb-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="Attach files"
            className={`h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted/40 hover:text-foreground ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
            onClick={onClick}
            disabled={disabled}
          >
            <IconPaperclip className="h-4 w-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Attach files</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function toContextItems(
  attachments: FileAttachment[],
  onRemove: (id: string) => void,
): ContextItem[] {
  return attachments.map((att) =>
    att.isImage
      ? ({
          kind: "image" as const,
          id: `image:${att.id}`,
          label: `Image (${formatBytes(att.size)})`,
          attachment: att,
          onRemove: () => onRemove(att.id),
        } as ImageContextItem)
      : ({
          kind: "file-attachment" as const,
          id: `file:${att.id}`,
          label: att.fileName,
          attachment: att,
          onRemove: () => onRemove(att.id),
        } as FileAttachmentContextItem),
  );
}
