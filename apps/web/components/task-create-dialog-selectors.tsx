/* eslint-disable max-lines -- groups all create-dialog selector subcomponents; splitting per-selector files is a separate refactor. */
"use client";

import { useEffect, useLayoutEffect, useRef, useState, memo, useCallback, useMemo } from "react";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPaperclip } from "@tabler/icons-react";
import { Combobox } from "./combobox";
import { scoreBranch } from "@/lib/utils/branch-filter";
import { BranchRefreshButton } from "./branch-refresh-button";
import {
  processFile,
  formatBytes,
  MAX_FILES,
  MAX_TOTAL_SIZE,
  type FileAttachment,
} from "@/components/task/chat/file-attachment";
import { ContextZone } from "@/components/task/chat/context-items/context-zone";
import { MentionMenu } from "@/components/task/chat/mention-menu";
import type { ContextItem, ImageContextItem, FileAttachmentContextItem } from "@/lib/types/context";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { JiraImportBar } from "@/components/jira/jira-import-bar";
import { LinearImportBar } from "@/components/linear/linear-import-bar";
import { VoiceInputButton } from "@/components/task/chat/voice-input-button";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { useTaskCreatePromptMention } from "@/hooks/use-task-create-prompt-mention";

const CURSOR_POINTER_CLASS = "cursor-pointer";

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
      dropdownLabel="Repository"
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      testId="repository-selector"
    />
  );
});

type BranchOption = {
  value: string;
  label: string;
  keywords?: string[];
  renderLabel: () => React.ReactNode;
};

type BranchSelectorProps = {
  options: BranchOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  searchPlaceholder: string;
  emptyMessage: string;
  triggerClassName?: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  fetchedAt?: string;
  fetchError?: string;
  loading?: boolean;
};

export const BranchSelector = memo(function BranchSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
  triggerClassName,
  onRefresh,
  refreshing,
  fetchedAt,
  fetchError,
  loading,
}: BranchSelectorProps) {
  const headerAction = onRefresh ? (
    <BranchRefreshButton
      onRefresh={onRefresh}
      refreshing={refreshing}
      fetchedAt={fetchedAt}
      fetchError={fetchError}
    />
  ) : undefined;
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      disabled={disabled}
      dropdownLabel="Base Branch"
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      testId="branch-selector"
      filter={scoreBranch}
      headerAction={headerAction}
      loading={loading}
    />
  );
});

type AgentSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
};

export const AgentSelector = memo(function AgentSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: AgentSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder="Search agents..."
      emptyMessage="No agent found."
      disabled={disabled}
      dropdownLabel="Agent Profile"
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={`min-w-0 ${triggerClassName ?? ""}`}
      testId="agent-profile-selector"
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
};

export const ExecutorSelector = memo(function ExecutorSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: ExecutorSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      emptyMessage="No executor found."
      disabled={disabled}
      dropdownLabel="Executor"
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      showSearch={false}
    />
  );
});

type ExecutorProfileSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
};

export const ExecutorProfileSelector = memo(function ExecutorProfileSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: ExecutorProfileSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder="Search profiles..."
      emptyMessage="No profile found."
      disabled={disabled}
      dropdownLabel="Executor Profile"
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={`min-w-0 ${triggerClassName ?? ""}`}
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
  const inputRef = useRef<HTMLInputElement>(null);
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
      onChange={(e) => onChange(e.target.value)}
      placeholder="Task name"
      data-testid="task-title-input"
      className="w-full min-w-0 max-w-full border border-input bg-input/20 dark:bg-input/30 text-sm font-medium rounded-md px-3 py-2 placeholder:text-muted-foreground/70 outline-none focus-visible:border-ring transition-colors"
    />
  );
});

// Memoized description input to prevent re-rendering the entire dialog on every keystroke
type TaskFormInputsProps = {
  isSessionMode: boolean;
  autoFocus?: boolean;
  initialDescription: string;
  onDescriptionChange: (hasContent: boolean) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
  disabled?: boolean;
  placeholder?: string;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
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
   * Called after a non-empty voice transcript was inserted into the description
   * when the user has voice auto-send enabled. The dialog wires this to a
   * programmatic form submit so dictation can create the task hands-free.
   */
  onVoiceAutoSend?: () => void;
};

function useFileAttachments() {
  const [attachments, setAttachments] = useState<FileAttachment[]>([]);
  const [isDragging, setIsDragging] = useState(false);

  const addFiles = useCallback(async (files: File[]) => {
    const processed: FileAttachment[] = [];
    for (const file of files) {
      const attachment = await processFile(file);
      if (attachment) processed.push(attachment);
    }
    if (processed.length === 0) return;

    setAttachments((prev) => {
      let nextCount = prev.length;
      let nextTotalSize = prev.reduce((sum, att) => sum + att.size, 0);
      const accepted: FileAttachment[] = [];
      for (const att of processed) {
        if (nextCount >= MAX_FILES) break;
        if (nextTotalSize + att.size > MAX_TOTAL_SIZE) break;
        accepted.push(att);
        nextCount += 1;
        nextTotalSize += att.size;
      }
      if (nextCount >= MAX_FILES && accepted.length < processed.length) {
        console.warn(`Maximum ${MAX_FILES} files allowed`);
      } else if (accepted.length < processed.length) {
        console.warn(`Total attachment size limit exceeded (max: ${formatBytes(MAX_TOTAL_SIZE)})`);
      }
      return accepted.length > 0 ? [...prev, ...accepted] : prev;
    });
  }, []);

  const handleRemoveAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((att) => att.id !== id));
  }, []);

  return { attachments, isDragging, setIsDragging, addFiles, handleRemoveAttachment };
}

function useAttachmentHandlers(
  disabled: boolean | undefined,
  addFiles: (files: File[]) => Promise<void>,
  setIsDragging: (v: boolean) => void,
) {
  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      if (disabled) return;
      const items = e.clipboardData?.items;
      if (!items) return;
      const files: File[] = [];
      for (const item of items) {
        if (item.kind === "file") {
          const file = item.getAsFile();
          if (file) files.push(file);
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

function AttachButton({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
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

function useDescriptionInput(
  initialDescription: string,
  autoFocus: boolean | undefined,
  descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>,
  onDescriptionChange: (hasContent: boolean) => void,
  attachments: FileAttachment[],
) {
  const [description, setDescription] = useState(initialDescription);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  // Caret offset to restore after a non-typed value mutation (e.g. voice
  // transcript splice). Consumed inside useLayoutEffect so the cursor lands
  // before the next paint and the user sees no jump.
  const pendingCursorRef = useRef<number | null>(null);

  useEffect(() => {
    const ref = descriptionValueRef as React.MutableRefObject<TaskFormInputsHandle | null>;
    if (ref) {
      ref.current = {
        getValue: () => description,
        setValue: (v: string) => {
          setDescription(v);
          onDescriptionChange(v.trim().length > 0);
        },
        getAttachments: () => attachments,
      };
    }
  }, [description, attachments, descriptionValueRef, onDescriptionChange]);

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

  const setDescriptionValue = useCallback(
    (newValue: string) => {
      const hadContent = description.trim().length > 0;
      const hasContent = newValue.trim().length > 0;
      setDescription(newValue);
      if (hadContent !== hasContent) onDescriptionChange(hasContent);
    },
    [description, onDescriptionChange],
  );

  const insertAtCursor = useCallback(
    (text: string) => {
      const trimmed = text.trim();
      if (!trimmed) return;
      const textarea = textareaRef.current;
      const start = textarea?.selectionStart ?? description.length;
      const end = textarea?.selectionEnd ?? description.length;
      const charBefore = start > 0 ? description.charAt(start - 1) : "";
      const needsLeadingSpace = charBefore !== "" && !/\s/.test(charBefore);
      const insert = needsLeadingSpace ? ` ${trimmed}` : trimmed;
      const next = description.slice(0, start) + insert + description.slice(end);
      pendingCursorRef.current = start + insert.length;
      setDescriptionValue(next);
    },
    [description, setDescriptionValue],
  );

  return { description, textareaRef, setDescriptionValue, insertAtCursor };
}

type FormInputsToolbarProps = {
  onAttach: () => void;
  disabled?: boolean;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
  jiraImport?: TaskFormInputsProps["jiraImport"];
  linearImport?: TaskFormInputsProps["linearImport"];
  voice?: {
    onTranscript: (text: string) => void;
    onAutoSend?: () => void;
  };
};

function FormInputsToolbar({
  onAttach,
  disabled,
  onEnhancePrompt,
  isEnhancingPrompt,
  isUtilityConfigured,
  jiraImport,
  linearImport,
  voice,
}: FormInputsToolbarProps) {
  return (
    <div className="flex items-center px-1 pb-1">
      <AttachButton onClick={onAttach} disabled={disabled} />
      {onEnhancePrompt && (
        <EnhancePromptButton
          onClick={onEnhancePrompt}
          isLoading={isEnhancingPrompt ?? false}
          isConfigured={isUtilityConfigured}
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
      {voice && (
        <div className="ml-auto flex items-center">
          <VoiceInputButton
            onTranscript={voice.onTranscript}
            onAutoSend={voice.onAutoSend}
            disabled={disabled}
          />
        </div>
      )}
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

function useTextareaHandlers(
  mention: ReturnType<typeof useTaskCreatePromptMention>,
  onKeyDown: TaskFormInputsProps["onKeyDown"],
) {
  const { handleChange: mentionHandleChange, handleKeyDown: mentionHandleKeyDown } = mention;
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => mentionHandleChange(e.target.value),
    [mentionHandleChange],
  );
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      mentionHandleKeyDown(e);
      if (e.defaultPrevented) return;
      onKeyDown?.(e);
    },
    [mentionHandleKeyDown, onKeyDown],
  );
  return { handleChange, handleKeyDown };
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
  if (!isDragging) return null;
  return (
    <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
      <span className="text-sm text-primary font-medium">Drop files here</span>
    </div>
  );
}

export const TaskFormInputs = memo(function TaskFormInputs({
  isSessionMode,
  autoFocus,
  initialDescription,
  onDescriptionChange,
  onKeyDown,
  descriptionValueRef,
  disabled,
  placeholder,
  onEnhancePrompt,
  isEnhancingPrompt,
  isUtilityConfigured,
  jiraImport,
  linearImport,
  onVoiceAutoSend,
}: TaskFormInputsProps) {
  const { attachments, isDragging, setIsDragging, addFiles, handleRemoveAttachment } =
    useFileAttachments();
  const { handlePaste, handleDragOver, handleDragLeave, handleDrop } = useAttachmentHandlers(
    disabled,
    addFiles,
    setIsDragging,
  );
  const contextItems = useMemo(
    () => toContextItems(attachments, handleRemoveAttachment),
    [attachments, handleRemoveAttachment],
  );
  const { description, textareaRef, setDescriptionValue, insertAtCursor } = useDescriptionInput(
    initialDescription,
    autoFocus,
    descriptionValueRef,
    onDescriptionChange,
    attachments,
  );
  const mention = useTaskCreatePromptMention({
    textareaRef,
    value: description,
    onChange: setDescriptionValue,
  });
  const { handleChange, handleKeyDown } = useTextareaHandlers(mention, onKeyDown);
  const { fileInputRef, handleAttachClick, handleFileInputChange } = useFileInputClick(addFiles);
  const voiceBinding = useMemo(
    () => ({ onTranscript: insertAtCursor, onAutoSend: onVoiceAutoSend }),
    [insertAtCursor, onVoiceAutoSend],
  );

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
        <ContextZone items={contextItems} />
        <Textarea
          ref={textareaRef}
          placeholder={
            placeholder ??
            (isSessionMode
              ? "Describe what you want the agent to do... (@ to insert a saved prompt)"
              : "Write a prompt for the agent... (@ to insert a saved prompt)")
          }
          value={description}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          data-testid="task-description-input"
          rows={2}
          className={`min-w-0 max-w-full field-sizing-fixed wrap-anywhere border-0 focus-visible:ring-0 focus-visible:ring-offset-0 ${isSessionMode ? "min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]" : "min-h-[96px] max-h-[240px] resize-y overflow-auto text-[13px]"}`}
          required={isSessionMode}
          disabled={disabled}
        />
        <FormInputsToolbar
          onAttach={handleAttachClick}
          disabled={disabled}
          onEnhancePrompt={onEnhancePrompt}
          isEnhancingPrompt={isEnhancingPrompt}
          isUtilityConfigured={isUtilityConfigured}
          jiraImport={jiraImport}
          linearImport={linearImport}
          voice={voiceBinding}
        />
        <HiddenFileInput inputRef={fileInputRef} onChange={handleFileInputChange} />
      </div>
      <PromptMentionPopover mention={mention} />
      <DraggingOverlay isDragging={isDragging} />
    </div>
  );
});
