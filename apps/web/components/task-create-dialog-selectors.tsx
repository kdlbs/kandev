/* eslint-disable max-lines -- groups all create-dialog selector subcomponents; splitting per-selector files is a separate refactor. */
"use client";

import {
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  memo,
  useCallback,
  useMemo,
} from "react";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPaperclip } from "@tabler/icons-react";
import { Combobox } from "./combobox";
import type { ComboboxOption } from "./combobox";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { IconCheck, IconChevronDown, IconX } from "@tabler/icons-react";
import { formatBytes } from "@/lib/utils/format-bytes";
import {
  processFile,
  MAX_FILES,
  MAX_TOTAL_SIZE,
  type FileAttachment,
} from "@/components/task/chat/file-attachment";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "@/components/task/chat/use-attachment-file-feedback";
import {
  readClipboardAttachments,
  type ImagePasteIssue,
} from "@/components/task/chat/clipboard-attachments";
import { ContextZone } from "@/components/task/chat/context-items/context-zone";
import { MentionMenu } from "@/components/task/chat/mention-menu";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";
import { TaskPromptReferenceEditor } from "@/components/task-prompt-reference-editor";
import type { TaskCreateLaunchPreview } from "@/components/task-create-dialog-launch-preview";
import { composeLaunchPreviewPrompt } from "@/components/task-create-dialog-launch-preview";
import {
  TaskCreateLaunchPreviewContent,
  TaskCreateLaunchPreviewToggle,
} from "@/components/task-create-dialog-launch-preview-control";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { JiraImportBar } from "@/components/jira/jira-import-bar";
import { LinearImportBar } from "@/components/linear/linear-import-bar";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { useTaskCreatePromptMention } from "@/hooks/use-task-create-prompt-mention";
import { cn } from "@/lib/utils";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { ApiError } from "@/lib/api/client";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { TaskCreateDialogTaskCreatedContext } from "./task-create-dialog-task-created";
import {
  composerIdentity,
  composerInsertionText,
  useStablePluginComposerCapability,
} from "@/lib/plugins/composer-capability";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { ChatInputPluginActions } from "@/components/task/chat/chat-input-plugin-actions";

export { BranchSelector } from "./branch-selector";
export type { BranchOption, BranchSelectorProps } from "./branch-selector";

const CURSOR_POINTER_CLASS = "cursor-pointer";

type AttachmentLimitRejection = "count" | "total-size" | null;

function acceptAttachmentsWithinLimits(
  existing: FileAttachment[],
  processed: FileAttachment[],
): { accepted: FileAttachment[]; rejection: AttachmentLimitRejection } {
  let nextCount = existing.length;
  let nextTotalSize = existing.reduce((sum, att) => sum + att.size, 0);
  const accepted: FileAttachment[] = [];

  for (const att of processed) {
    if (nextCount >= MAX_FILES) return { accepted, rejection: "count" };
    if (nextTotalSize + att.size > MAX_TOTAL_SIZE) {
      return { accepted, rejection: "total-size" };
    }
    accepted.push(att);
    nextCount += 1;
    nextTotalSize += att.size;
  }

  return { accepted, rejection: null };
}

type RepositoryOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type RepositorySelectorProps = {
  options: RepositoryOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  searchPlaceholder: string;
  emptyMessage: string;
  triggerClassName?: string;
};

export const RepositorySelector = memo(function RepositorySelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
  triggerClassName,
}: RepositorySelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      disabled={disabled}
      dropdownLabel={t("task:repository2")}
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      testId="repository-selector"
    />
  );
});

type AgentSelectorProps = {
  options: Array<{
    value: string;
    label: string;
    renderLabel: () => React.ReactNode;
    renderTriggerLabel?: () => React.ReactNode;
  }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
  popoverPortal?: boolean;
  touchTarget?: boolean;
  testId?: string;
  triggerId?: string;
  ariaDescribedBy?: string;
};

export const AgentSelector = memo(function AgentSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
  popoverPortal,
  touchTarget,
  testId,
  triggerId,
  ariaDescribedBy,
}: AgentSelectorProps) {
  const { t } = useTranslation();
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={t("task:searchAgents")}
      emptyMessage={t("task:noAgentFound")}
      disabled={disabled}
      dropdownLabel={t("task:agentProfile2")}
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={cn("min-w-0", triggerClassName)}
      popoverPortal={popoverPortal}
      touchTarget={touchTarget}
      testId={testId ?? "agent-profile-selector"}
      triggerId={triggerId}
      ariaDescribedBy={ariaDescribedBy}
    />
  );
});

type ExecutorSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
  popoverPortal?: boolean;
};

export const ExecutorSelector = memo(function ExecutorSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
  popoverPortal,
}: ExecutorSelectorProps) {
  const { t } = useTranslation();
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      emptyMessage={t("task:noExecutorFound")}
      disabled={disabled}
      dropdownLabel={t("task:executor2")}
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      popoverPortal={popoverPortal}
      showSearch={false}
    />
  );
});

type ExecutorProfileSelectorProps = {
  options: ComboboxOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
  popoverPortal?: boolean;
};

function TouchExecutorProfileSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: ExecutorProfileSelectorProps) {
  const { t } = useTranslation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const selected = options.find((option) => option.value === value);
  return (
    <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
      <DrawerTrigger asChild>
        <Button
          variant="ghost"
          role="combobox"
          aria-label={t("task:executorProfile2")}
          aria-expanded={drawerOpen}
          aria-haspopup="dialog"
          disabled={disabled}
          data-testid="executor-profile-selector"
          className={cn("h-11 min-h-12 w-full justify-between", triggerClassName)}
        >
          <span className="min-w-0 truncate text-left">
            {selected?.renderTriggerLabel?.() ??
              selected?.renderLabel?.() ??
              selected?.label ??
              placeholder}
          </span>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </DrawerTrigger>
      <DrawerContent className="flex h-[min(82dvh,40rem)] max-h-[calc(100dvh-env(safe-area-inset-bottom,0px))] flex-col overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
        <DrawerHeader className="shrink-0 pr-12 text-left">
          <DrawerTitle>{t("task:executorProfile2")}</DrawerTitle>
          <DrawerDescription>{placeholder}</DrawerDescription>
          <DrawerClose asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("common:close")}
              className="absolute right-3 top-3 min-h-12 min-w-12"
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <Command
          className="min-h-0 flex-1 border-t"
          data-testid="executor-profile-selector-dropdown"
        >
          <CommandInput placeholder={t("task:searchProfiles")} className="h-11 min-h-12" />
          <CommandList className="min-h-0 flex-1 overflow-y-auto" data-vaul-no-drag>
            <CommandEmpty>{t("task:noProfileFound")}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  keywords={[option.label, option.description ?? ""]}
                  disabled={option.disabled}
                  className="min-h-12 py-2"
                  onSelect={() => {
                    if (option.disabled) return;
                    onValueChange(option.value);
                    setDrawerOpen(false);
                  }}
                >
                  <span className="flex min-w-0 flex-1 items-center">
                    {option.renderLabel ? option.renderLabel() : option.label}
                  </span>
                  <IconCheck
                    className={cn(
                      "ml-2 h-4 w-4 shrink-0",
                      option.value === value ? "opacity-100" : "opacity-0",
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </DrawerContent>
    </Drawer>
  );
}

export const ExecutorProfileSelector = memo(function ExecutorProfileSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
  popoverPortal,
}: ExecutorProfileSelectorProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  if (usesTouchDrawer) {
    return (
      <TouchExecutorProfileSelector
        options={options}
        value={value}
        onValueChange={onValueChange}
        disabled={disabled}
        placeholder={placeholder}
        triggerClassName={triggerClassName}
      />
    );
  }
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={t("task:searchProfiles")}
      emptyMessage={t("task:noProfileFound")}
      disabled={disabled}
      dropdownLabel={t("task:executorProfile2")}
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={cn("min-w-0", triggerClassName)}
      popoverPortal={popoverPortal}
      testId="executor-profile-selector"
    />
  );
});

type InlineTaskNameProps = {
  value: string;
  onChange: (value: string) => void;
  autoFocus?: boolean;
};

export const InlineTaskName = memo(function InlineTaskName({
  value,
  onChange,
  autoFocus,
}: InlineTaskNameProps) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(value);
  const hasFocusedRef = useRef(false);

  useEffect(() => {
    if (autoFocus && !hasFocusedRef.current && inputRef.current) {
      hasFocusedRef.current = true;
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [autoFocus]);

  return (
    <input
      ref={inputRef}
      type="text"
      value={value}
      onChange={(e) => onChange(clampChange(e))}
      placeholder={t("task:taskName")}
      data-testid="task-title-input"
      className="w-full min-w-0 max-w-full border border-input bg-input/20 dark:bg-input/30 text-sm font-medium rounded-md px-3 py-2 placeholder:text-muted-foreground/70 outline-none focus-visible:border-ring transition-colors"
    />
  );
});

// Memoized description input to prevent re-rendering the entire dialog on every keystroke
type TaskFormInputsProps = {
  isSessionMode: boolean;
  promptReferencesEnabled?: boolean;
  taskId?: string | null;
  workspaceId?: string | null;
  autoFocus?: boolean;
  initialDescription: string;
  onDescriptionChange: (hasContent: boolean) => void;
  onDescriptionValueChange?: (value: string) => void;
  initialAttachments?: FileAttachment[];
  onAttachmentsChange?: (attachments: FileAttachment[]) => boolean | void;
  quickChatComposer?: boolean;
  composerSubmitDisabled?: boolean;
  toolbarActions?: React.ReactNode;
  toolbarLeadingActions?: React.ReactNode;
  toolbarAttachmentActions?: React.ReactNode;
  contextLeadingContent?: React.ReactNode;
  onPendingAttachmentUploadsChange?: (pending: boolean) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
  disabled?: boolean;
  placeholder?: string;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
  launchPreview?: TaskCreateLaunchPreview | null;
  jiraImport?: {
    workspaceId: string | null;
    disabled?: boolean;
    onImport: (ticket: JiraTicket) => void;
  };
  linearImport?: {
    workspaceId: string | null;
    disabled?: boolean;
    onImport: (issue: LinearIssue) => void;
  };
  /**
   * Submits the form the way the native submit control does, for a plugin
   * composer action that finished producing text (dictation, for instance).
   * The dialog wires this to its own submit handler, so validation, gating
   * and error handling stay native.
   */
  onComposerSubmit?: () => boolean | Promise<boolean>;
};

// eslint-disable-next-line max-lines-per-function
function useFileAttachments(
  workspaceId: string | null | undefined,
  onPendingAttachmentUploadsChange?: (pending: boolean) => void,
  initialAttachments: FileAttachment[] = [],
  onAttachmentsChange?: (attachments: FileAttachment[]) => boolean | void,
) {
  const [attachments, setAttachments] = useState<FileAttachment[]>(initialAttachments);
  const attachmentsRef = useRef<FileAttachment[]>(initialAttachments);

  const updateAttachment = useCallback(
    (id: string, update: Partial<FileAttachment>): boolean => {
      const next = attachmentsRef.current.map((attachment) =>
        attachment.id === id ? { ...attachment, ...update } : attachment,
      );
      attachmentsRef.current = next;
      setAttachments(next);
      return onAttachmentsChange?.(next) !== false;
    },
    [onAttachmentsChange],
  );

  const uploadPendingAttachment = useCallback(
    async (attachment: FileAttachment) => {
      if (!workspaceId || !attachment.file || attachment.attachmentId) return;
      if (!updateAttachment(attachment.id, { uploadStatus: "uploading" })) return;
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
        const keptByDraft = updateAttachment(attachment.id, {
          attachmentId: uploaded.attachment_id,
          expiresAt: uploaded.expires_at,
          uploadStatus: "ready",
          size: uploaded.size_bytes,
        });
        if (!keptByDraft) void deleteAttachment(uploaded.attachment_id).catch(() => undefined);
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

  const [isDragging, setIsDragging] = useState(false);
  const warnAttachmentCountLimit = useAttachmentCountFeedback();
  const rejectOversizedFile = useAttachmentFileFeedback();
  const warnAttachmentTotalSizeLimit = useAttachmentTotalSizeFeedback();
  const warnUnreadablePastedImage = useUnreadablePastedImageFeedback();

  useEffect(() => {
    onPendingAttachmentUploadsChange?.(
      attachments.some((attachment) => attachment.file && !attachment.attachmentId),
    );
  }, [attachments, onPendingAttachmentUploadsChange, workspaceId]);

  const addFiles = useCallback(
    async (files: File[], issue?: ImagePasteIssue) => {
      if (issue === "unreadable-image") {
        warnUnreadablePastedImage();
        return;
      }
      const processed: FileAttachment[] = [];
      for (const file of files) {
        if (rejectOversizedFile(file)) continue;
        const attachment = await processFile(file);
        if (attachment) processed.push(attachment);
      }
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
      for (const attachment of accepted) void uploadPendingAttachment(attachment);
    },
    [
      rejectOversizedFile,
      warnAttachmentCountLimit,
      warnAttachmentTotalSizeLimit,
      warnUnreadablePastedImage,
      uploadPendingAttachment,
    ],
  );

  const handleRemoveAttachment = useCallback((id: string) => {
    const removed = attachmentsRef.current.find((attachment) => attachment.id === id);
    const next = attachmentsRef.current.filter((att) => att.id !== id);
    attachmentsRef.current = next;
    setAttachments(next);
    if (removed?.attachmentId) void deleteAttachment(removed.attachmentId).catch(() => undefined);
  }, []);

  const clearAttachments = useCallback(() => {
    const current = attachmentsRef.current;
    attachmentsRef.current = [];
    setAttachments([]);
    current.forEach((attachment) => {
      if (attachment.attachmentId)
        void deleteAttachment(attachment.attachmentId).catch(() => undefined);
    });
  }, []);

  const handleRetryAttachment = useCallback(
    (id: string) => {
      const attachment = attachmentsRef.current.find((item) => item.id === id);
      if (attachment) void uploadPendingAttachment(attachment);
    },
    [uploadPendingAttachment],
  );

  return {
    attachments,
    isDragging,
    setIsDragging,
    addFiles,
    handleRemoveAttachment,
    handleRetryAttachment,
    clearAttachments,
  };
}

function useAttachmentHandlers(
  disabled: boolean | undefined,
  addFiles: (files: File[], issue?: ImagePasteIssue) => Promise<void>,
  setIsDragging: (v: boolean) => void,
) {
  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLElement>) => {
      if (disabled) return;
      const { files, issue } = readClipboardAttachments(e.clipboardData);
      if (files.length > 0 || issue) {
        e.preventDefault();
        void addFiles(files, issue);
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
    [disabled, setIsDragging],
  );

  const handleDragLeave = useCallback(
    (e: React.DragEvent) => {
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
    },
    [setIsDragging],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);
      if (disabled) return;
      const files = Array.from(e.dataTransfer.files).filter((f) => f.size > 0 || f.type !== "");
      if (files.length > 0) {
        void addFiles(files);
      }
    },
    [disabled, addFiles, setIsDragging],
  );

  return { handlePaste, handleDragOver, handleDragLeave, handleDrop };
}

function toContextItems(
  attachments: FileAttachment[],
  onRemove: (id: string) => void,
  onRetry: (id: string) => void,
): ContextItem[] {
  return attachments.map((att) =>
    att.isImage
      ? ({
          kind: "image" as const,
          id: `image:${att.id}`,
          label: t("task:imageWithSize", { bytes: formatBytes(att.size) }),
          attachment: att,
          onRemove: () => onRemove(att.id),
          onRetry: () => onRetry(att.id),
        } as ImageContextItem)
      : ({
          kind: "file-attachment" as const,
          id: `file:${att.id}`,
          label: att.fileName,
          attachment: att,
          onRemove: () => onRemove(att.id),
          onRetry: () => onRetry(att.id),
        } as FileAttachmentContextItem),
  );
}

function AttachButton({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const isMobile = useResponsiveBreakpoint().isMobile;
  const usesTouchTarget = usesTouchDrawer || isMobile;
  return (
    <div className="flex items-center">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("task:attachFiles")}
            className={`inline-flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted/40 hover:text-foreground ${usesTouchTarget ? "h-11 w-11" : "h-7 w-7"} ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
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

type DescriptionInputOptions = {
  initialDescription: string;
  autoFocus: boolean | undefined;
  descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
  onDescriptionChange: (hasContent: boolean) => void;
  onDescriptionValueChange?: (value: string) => void;
  attachments: FileAttachment[];
  clearAttachments: () => void;
};

function useDescriptionInput({
  initialDescription,
  autoFocus,
  descriptionValueRef,
  onDescriptionChange,
  onDescriptionValueChange,
  attachments,
  clearAttachments,
}: DescriptionInputOptions) {
  const [description, setDescription] = useState(initialDescription);
  const descriptionRef = useRef(initialDescription);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  // Caret offset to restore after a non-typed value mutation (e.g. a plugin
  // transcript splice). Consumed inside useLayoutEffect so the cursor lands
  // before the next paint and the user sees no jump.
  const pendingCursorRef = useRef<number | null>(null);

  const setDescriptionValue = useCallback(
    (newValue: string) => {
      const hadContent = descriptionRef.current.trim().length > 0;
      const hasContent = newValue.trim().length > 0;
      descriptionRef.current = newValue;
      setDescription(newValue);
      onDescriptionValueChange?.(newValue);
      if (hadContent !== hasContent) onDescriptionChange(hasContent);
    },
    [onDescriptionChange, onDescriptionValueChange],
  );

  useEffect(() => {
    const ref = descriptionValueRef as React.MutableRefObject<TaskFormInputsHandle | null>;
    if (ref) {
      ref.current = {
        getValue: () => descriptionRef.current,
        setValue: setDescriptionValue,
        getAttachments: () => attachments,
        clearAttachments,
      };
    }
  }, [attachments, clearAttachments, descriptionValueRef, setDescriptionValue]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  useLayoutEffect(() => {
    const pos = pendingCursorRef.current;
    if (pos === null) return;
    pendingCursorRef.current = null;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    textarea.setSelectionRange(pos, pos);
  }, [description]);

  useEffect(() => {
    if (!autoFocus) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  }, [autoFocus]);

  const insertAtCursor = useCallback(
    (text: string) => {
      // Read the synchronous ref, not the render snapshot: a plugin can
      // insert twice (or insert then submit) inside one callback, before React
      // has re-rendered with the first insertion.
      const current = descriptionRef.current;
      const textarea = textareaRef.current;
      // A caret we set but have not applied yet outranks the DOM's: after a
      // programmatic insertion the textarea still reports the pre-insert
      // selection until the layout effect below runs, so a second insertion in
      // the same callback would splice at the old offset.
      const pending = pendingCursorRef.current;
      const start = pending ?? textarea?.selectionStart ?? current.length;
      const end = pending ?? textarea?.selectionEnd ?? current.length;
      const insert = composerInsertionText(text, start > 0 ? current.charAt(start - 1) : "");
      if (!insert) return;
      const next = current.slice(0, start) + insert + current.slice(end);
      pendingCursorRef.current = start + insert.length;
      setDescriptionValue(next);
    },
    [setDescriptionValue],
  );

  return { description, descriptionRef, textareaRef, setDescriptionValue, insertAtCursor };
}

type FormInputsToolbarProps = {
  onAttach: () => void;
  disabled?: boolean;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
  launchPreview?: TaskCreateLaunchPreview | null;
  isLaunchPromptPreview: boolean;
  onToggleLaunchPromptPreview: () => void;
  jiraImport?: TaskFormInputsProps["jiraImport"];
  linearImport?: TaskFormInputsProps["linearImport"];
  pluginActions?: React.ReactNode;
  toolbarActions?: React.ReactNode;
  toolbarLeadingActions?: React.ReactNode;
  toolbarAttachmentActions?: React.ReactNode;
};

function FormInputsToolbar({
  onAttach,
  disabled,
  onEnhancePrompt,
  isEnhancingPrompt,
  isUtilityConfigured,
  launchPreview,
  isLaunchPromptPreview,
  onToggleLaunchPromptPreview,
  jiraImport,
  linearImport,
  pluginActions,
  toolbarActions,
  toolbarLeadingActions,
  toolbarAttachmentActions,
}: FormInputsToolbarProps) {
  return (
    <div className="flex items-center p-1">
      {toolbarLeadingActions}
      <AttachButton onClick={onAttach} disabled={disabled} />
      {toolbarAttachmentActions}
      {onEnhancePrompt && (
        <EnhancePromptButton
          onClick={onEnhancePrompt}
          isLoading={isEnhancingPrompt ?? false}
          isConfigured={isUtilityConfigured}
        />
      )}
      {launchPreview?.stepPrompt.trim() && (
        <TaskCreateLaunchPreviewToggle
          active={isLaunchPromptPreview}
          disabled={disabled}
          stepName={launchPreview.stepName}
          onToggle={onToggleLaunchPromptPreview}
        />
      )}
      {jiraImport && (
        <JiraImportBar
          workspaceId={jiraImport.workspaceId}
          disabled={jiraImport.disabled}
          onImport={jiraImport.onImport}
        />
      )}
      {linearImport && (
        <LinearImportBar
          workspaceId={linearImport.workspaceId}
          disabled={linearImport.disabled}
          onImport={linearImport.onImport}
        />
      )}
      <div className="ml-auto flex items-center gap-1">
        {pluginActions}
        {toolbarActions}
      </div>
    </div>
  );
}

function PromptMentionPopover({
  mention,
}: {
  mention: ReturnType<typeof useTaskCreatePromptMention>;
}) {
  return (
    <MentionMenu
      isOpen={mention.isOpen}
      isLoading={mention.isLoading}
      position={mention.position}
      items={mention.items}
      query={mention.query}
      selectedIndex={mention.selectedIndex}
      onSelect={mention.handleSelect}
      onClose={mention.closeMenu}
      setSelectedIndex={mention.setSelectedIndex}
    />
  );
}

function useCreationComposerPluginActions(args: {
  isSessionMode: boolean;
  quickChatComposer?: boolean;
  composerSubmitDisabled?: boolean;
  taskId: string | null;
  disabled: boolean;
  description: string;
  descriptionRef: React.RefObject<string>;
  focusComposer: () => boolean;
  insertAtCursor: (text: string) => void;
  submit?: () => boolean | Promise<boolean>;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const registerTaskCreatedHandler = useContext(TaskCreateDialogTaskCreatedContext);
  const surface = getCreationComposerSurface(args);
  const composer = useStablePluginComposerCapability(
    {
      insertText: (text) => {
        args.insertAtCursor(text);
        return true;
      },
      focus: args.focusComposer,
      // Gate on the synchronous ref for the same reason the chat composer
      // reads its editor: insert-then-submit in one callback happens before
      // React re-renders with the new description.
      submit: async () => {
        if (
          args.disabled ||
          args.composerSubmitDisabled ||
          !args.descriptionRef.current.trim() ||
          !args.submit
        )
          return false;
        return await args.submit();
      },
    },
    composerIdentity(surface, args.taskId, null),
  );
  if (args.quickChatComposer) {
    return (
      <ChatInputPluginActions
        sessionId={null}
        taskId={null}
        surface="quick-chat"
        presentation={isMobile ? "mobile" : "desktop"}
        disabled={args.disabled || args.composerSubmitDisabled}
        submittable={
          !args.disabled && !args.composerSubmitDisabled && args.description.trim().length > 0
        }
        composer={composer}
      />
    );
  }
  return (
    <PluginSlot
      name={args.isSessionMode ? "new-session-input-actions" : "task-create-input-actions"}
      slotProps={{
        surface,
        presentation: isMobile ? "mobile" : "desktop",
        taskId: args.taskId,
        activeSessionId: null,
        sessionIds: [],
        disabled: args.disabled,
        submittable: !args.disabled && args.description.trim().length > 0,
        composer,
        ...(!args.isSessionMode && registerTaskCreatedHandler
          ? { registerTaskCreatedHandler }
          : {}),
      }}
      actionSurface={{ surface: "composer", presentation: isMobile ? "mobile" : "desktop" }}
    />
  );
}

function getCreationComposerSurface(args: {
  quickChatComposer?: boolean;
  isSessionMode: boolean;
}): "quick-chat" | "new-session" | "task-create" {
  if (args.quickChatComposer) return "quick-chat";
  if (args.isSessionMode) return "new-session";
  return "task-create";
}

function useTextareaHandlers(
  mention: ReturnType<typeof useTaskCreatePromptMention>,
  onKeyDown: TaskFormInputsProps["onKeyDown"],
) {
  const { handleChange: mentionHandleChange, handleKeyDown: mentionHandleKeyDown } = mention;
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) =>
      mentionHandleChange(e.target.value, e.target.selectionStart),
    [mentionHandleChange],
  );
  const handleKeyDownCapture = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => mentionHandleKeyDown(e),
    [mentionHandleKeyDown],
  );
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.defaultPrevented) return;
      onKeyDown?.(e);
    },
    [onKeyDown],
  );
  return { handleChange, handleKeyDownCapture, handleKeyDown };
}

function useFileInputClick(addFiles: (files: File[]) => Promise<void> | void) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const handleAttachClick = useCallback(() => fileInputRef.current?.click(), []);
  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (files && files.length > 0) void addFiles(Array.from(files));
      e.target.value = "";
    },
    [addFiles],
  );
  return { fileInputRef, handleAttachClick, handleFileInputChange };
}

function HiddenFileInput({
  inputRef,
  onChange,
}: {
  inputRef: React.RefObject<HTMLInputElement | null>;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}) {
  return (
    <input
      ref={inputRef}
      type="file"
      multiple
      className="hidden"
      onChange={onChange}
      tabIndex={-1}
    />
  );
}

function DraggingOverlay({ isDragging }: { isDragging: boolean }) {
  const { t } = useTranslation();
  if (!isDragging) return null;
  return (
    <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
      <span className="text-sm text-primary font-medium">{t("task:dropFilesHere")}</span>
    </div>
  );
}

type TaskDescriptionInputProps = {
  isLaunchPromptPreview: boolean;
  launchPromptPreview: string;
  promptReferencesEnabled: boolean;
  referenceInputRef: React.RefObject<RichTextInputHandle | null>;
  description: string;
  onDescriptionChange: (value: string) => void;
  descriptionPlaceholder: string;
  isSessionMode: boolean;
  autoFocus?: boolean;
  disabled?: boolean;
  onKeyDown: TaskFormInputsProps["onKeyDown"];
  onPaste: React.ClipboardEventHandler<HTMLElement>;
  handleChange: React.ChangeEventHandler<HTMLTextAreaElement>;
  handleKeyDownCapture: React.KeyboardEventHandler<HTMLTextAreaElement>;
  handleKeyDown: React.KeyboardEventHandler<HTMLTextAreaElement>;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
};

function TaskDescriptionInput({
  isLaunchPromptPreview,
  launchPromptPreview,
  promptReferencesEnabled,
  referenceInputRef,
  description,
  onDescriptionChange,
  descriptionPlaceholder,
  isSessionMode,
  autoFocus,
  disabled,
  onKeyDown,
  onPaste,
  handleChange,
  handleKeyDownCapture,
  handleKeyDown,
  textareaRef,
}: TaskDescriptionInputProps) {
  if (isLaunchPromptPreview) {
    return <TaskCreateLaunchPreviewContent content={launchPromptPreview} />;
  }

  if (promptReferencesEnabled) {
    return (
      <TaskPromptReferenceEditor
        ref={referenceInputRef}
        value={description}
        onChange={onDescriptionChange}
        placeholder={descriptionPlaceholder}
        autoFocus={autoFocus}
        disabled={disabled}
        onKeyDown={onKeyDown}
        onPaste={onPaste}
      />
    );
  }

  return (
    <Textarea
      ref={textareaRef}
      placeholder={descriptionPlaceholder}
      value={description}
      onChange={handleChange}
      onKeyDownCapture={handleKeyDownCapture}
      onKeyDown={handleKeyDown}
      onPaste={onPaste}
      data-testid="task-description-input"
      rows={2}
      className={`min-w-0 max-w-full field-sizing-fixed wrap-anywhere border-0 focus-visible:ring-0 focus-visible:ring-offset-0 ${isSessionMode ? "min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]" : "min-h-[96px] max-h-[240px] resize-y overflow-auto text-[13px]"}`}
      required={isSessionMode}
      disabled={disabled}
    />
  );
}

// The input coordinates existing attachment, mention and plugin controls in one field.
// eslint-disable-next-line max-lines-per-function
export const TaskFormInputs = memo(function TaskFormInputs({
  workspaceId,
  isSessionMode,
  promptReferencesEnabled = false,
  autoFocus,
  initialDescription,
  onDescriptionChange,
  onDescriptionValueChange,
  initialAttachments,
  onAttachmentsChange,
  quickChatComposer,
  contextLeadingContent,
  composerSubmitDisabled,
  toolbarActions,
  toolbarLeadingActions,
  toolbarAttachmentActions,
  onPendingAttachmentUploadsChange,
  onKeyDown,
  descriptionValueRef,
  disabled,
  placeholder,
  onEnhancePrompt,
  isEnhancingPrompt,
  isUtilityConfigured,
  launchPreview,
  jiraImport,
  linearImport,
  onComposerSubmit,
  taskId = null,
}: TaskFormInputsProps) {
  const { t } = useTranslation();
  const [isLaunchPromptPreview, setIsLaunchPromptPreview] = useState(false);
  const hasLaunchPromptPreview = Boolean(launchPreview?.stepPrompt.trim());
  useEffect(() => {
    if (!hasLaunchPromptPreview) setIsLaunchPromptPreview(false);
  }, [hasLaunchPromptPreview]);
  const {
    attachments,
    isDragging,
    setIsDragging,
    addFiles,
    handleRemoveAttachment,
    handleRetryAttachment,
    clearAttachments,
  } = useFileAttachments(
    workspaceId,
    onPendingAttachmentUploadsChange,
    initialAttachments,
    onAttachmentsChange,
  );
  const { handlePaste, handleDragOver, handleDragLeave, handleDrop } = useAttachmentHandlers(
    disabled,
    addFiles,
    setIsDragging,
  );
  const contextItems = useMemo(
    () => toContextItems(attachments, handleRemoveAttachment, handleRetryAttachment),
    [attachments, handleRemoveAttachment, handleRetryAttachment],
  );
  const { description, descriptionRef, textareaRef, setDescriptionValue, insertAtCursor } =
    useDescriptionInput({
      initialDescription,
      autoFocus,
      descriptionValueRef,
      onDescriptionChange,
      onDescriptionValueChange,
      attachments,
      clearAttachments,
    });
  useEffect(() => {
    onAttachmentsChange?.(attachments);
  }, [attachments, onAttachmentsChange]);
  const referenceInputRef = useRef<RichTextInputHandle | null>(null);
  const mention = useTaskCreatePromptMention({
    textareaRef,
    inputRef: promptReferencesEnabled ? referenceInputRef : undefined,
    value: description,
    onChange: setDescriptionValue,
    promptInsertMode: promptReferencesEnabled ? "reference" : "inline",
  });
  const { handleChange, handleKeyDownCapture, handleKeyDown } = useTextareaHandlers(
    mention,
    onKeyDown,
  );
  const { fileInputRef, handleAttachClick, handleFileInputChange } = useFileInputClick(addFiles);
  const descriptionPlaceholder =
    placeholder ??
    (isSessionMode ? t("task:describeWhatYouWantTheAgent") : t("task:writeAPromptForTheAgent"));
  const insertComposerText = useCallback(
    (text: string) => {
      if (!promptReferencesEnabled) {
        insertAtCursor(text);
        return;
      }
      const input = referenceInputRef.current;
      if (!input) return;
      const current = descriptionRef.current;
      const start = input.getSelectionStart();
      const end = input.getSelectionEnd();
      const insert = composerInsertionText(text, start > 0 ? current.charAt(start - 1) : "");
      if (!insert) return;
      input.insertText(insert, start, end);
    },
    [descriptionRef, insertAtCursor, promptReferencesEnabled],
  );
  const focusComposer = useCallback(() => {
    if (promptReferencesEnabled) {
      const input = referenceInputRef.current;
      if (!input) return false;
      input.focus();
      return true;
    }
    if (!textareaRef.current) return false;
    textareaRef.current.focus();
    return true;
  }, [promptReferencesEnabled, textareaRef]);
  useEffect(() => {
    if (!promptReferencesEnabled || !autoFocus) return;
    const input = referenceInputRef.current;
    if (!input) return;
    const end = input.getValue().length;
    input.focus();
    input.setSelectionRange(end, end);
  }, [autoFocus, promptReferencesEnabled]);
  const pluginActions = useCreationComposerPluginActions({
    isSessionMode,
    quickChatComposer,
    composerSubmitDisabled,
    taskId,
    disabled: Boolean(disabled),
    description,
    descriptionRef,
    focusComposer,
    insertAtCursor: insertComposerText,
    submit: onComposerSubmit,
  });
  const launchPromptPreview =
    hasLaunchPromptPreview && launchPreview
      ? composeLaunchPreviewPrompt(launchPreview.stepPrompt, description)
      : "";

  return (
    <div
      className="relative min-w-0 max-w-full"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <div
        className={`min-w-0 max-w-full rounded-md border border-input bg-transparent focus-within:ring-2 focus-within:ring-ring/30 ${contextItems.length > 0 ? "ring-0" : ""}`}
      >
        <ContextZone
          items={contextItems}
          leadingContent={contextLeadingContent}
          rowClassName={quickChatComposer ? "gap-2 px-3 py-2" : undefined}
          scrollable={!quickChatComposer}
        />
        <TaskDescriptionInput
          isLaunchPromptPreview={isLaunchPromptPreview}
          launchPromptPreview={launchPromptPreview}
          promptReferencesEnabled={promptReferencesEnabled}
          referenceInputRef={referenceInputRef}
          description={description}
          onDescriptionChange={setDescriptionValue}
          descriptionPlaceholder={descriptionPlaceholder}
          isSessionMode={isSessionMode}
          autoFocus={autoFocus}
          disabled={disabled}
          onKeyDown={onKeyDown}
          onPaste={handlePaste}
          handleChange={handleChange}
          handleKeyDownCapture={handleKeyDownCapture}
          handleKeyDown={handleKeyDown}
          textareaRef={textareaRef}
        />
        <FormInputsToolbar
          onAttach={handleAttachClick}
          disabled={disabled}
          onEnhancePrompt={onEnhancePrompt}
          isEnhancingPrompt={isEnhancingPrompt}
          isUtilityConfigured={isUtilityConfigured}
          launchPreview={launchPreview}
          isLaunchPromptPreview={isLaunchPromptPreview}
          onToggleLaunchPromptPreview={() => setIsLaunchPromptPreview((active) => !active)}
          jiraImport={jiraImport}
          linearImport={linearImport}
          pluginActions={pluginActions}
          toolbarActions={toolbarActions}
          toolbarLeadingActions={toolbarLeadingActions}
          toolbarAttachmentActions={toolbarAttachmentActions}
        />
        <HiddenFileInput inputRef={fileInputRef} onChange={handleFileInputChange} />
      </div>
      {!promptReferencesEnabled && <PromptMentionPopover mention={mention} />}
      <DraggingOverlay isDragging={isDragging} />
    </div>
  );
});
