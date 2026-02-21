"use client";

import { useEffect, useRef, useState, memo, useCallback } from "react";
import { Textarea } from "@kandev/ui/textarea";
import { Combobox } from "./combobox";

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
    />
  );
});

type BranchOption = {
  value: string;
  label: string;
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
};

export const BranchSelector = memo(function BranchSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
}: BranchSelectorProps) {
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
      dropdownLabel="Agent profile"
      className={`min-w-[380px]${disabled ? "" : ` ${CURSOR_POINTER_CLASS}`}`}
      triggerClassName={triggerClassName}
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
      triggerClassName={triggerClassName}
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

  useEffect(() => {
    if (autoFocus && inputRef.current) {
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
      placeholder="task-name"
      size={Math.max(value.length, 9)}
      className="bg-transparent border-none outline-none focus:ring-0 text-sm font-medium min-w-0 rounded-md px-1.5 py-0.5 -mx-1.5 hover:bg-muted focus:bg-muted transition-colors"
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
  descriptionValueRef: React.RefObject<{ getValue: () => string } | null>;
  disabled?: boolean;
  placeholder?: string;
};

export const TaskFormInputs = memo(function TaskFormInputs({
  isSessionMode,
  autoFocus,
  initialDescription,
  onDescriptionChange,
  onKeyDown,
  descriptionValueRef,
  disabled,
  placeholder,
}: TaskFormInputsProps) {
  const [description, setDescription] = useState(initialDescription);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const ref = descriptionValueRef as React.MutableRefObject<{ getValue: () => string } | null>;
    if (ref) {
      ref.current = { getValue: () => description };
    }
  }, [description, descriptionValueRef]);

  // Auto-resize textarea + optional auto-focus with cursor at end
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  useEffect(() => {
    if (!autoFocus) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  }, [autoFocus]);

  const handleDescriptionChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value;
      const hadContent = description.trim().length > 0;
      const hasContent = newValue.trim().length > 0;
      setDescription(newValue);
      if (hadContent !== hasContent) {
        onDescriptionChange(hasContent);
      }
    },
    [description, onDescriptionChange],
  );

  return (
    <div>
      <Textarea
        ref={textareaRef}
        placeholder={
          placeholder ??
          (isSessionMode
            ? "Describe what you want the agent to do..."
            : "Write a prompt for the agent...")
        }
        value={description}
        onChange={handleDescriptionChange}
        onKeyDown={onKeyDown}
        rows={2}
        className={
          isSessionMode
            ? "min-h-[120px] max-h-[240px] resize-none overflow-auto"
            : "min-h-[96px] max-h-[240px] resize-y overflow-auto"
        }
        required={isSessionMode}
        disabled={disabled}
      />
    </div>
  );
});
