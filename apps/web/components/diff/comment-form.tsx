"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Textarea } from "@kandev/ui/textarea";
import { IconChevronDown, IconPlayerPlay, IconSend, IconX } from "@tabler/icons-react";

interface CommentFormProps {
  initialContent?: string;
  onSubmit: (content: string) => void;
  onCancel: () => void;
  onSubmitAndRun?: (content: string) => void;
  isEditing?: boolean;
  autoFocus?: boolean;
}

function SplitAddButton({
  disabled,
  onSubmit,
  onSubmitAndRun,
}: {
  disabled: boolean;
  onSubmit: () => void;
  onSubmitAndRun: () => void;
}) {
  return (
    <div className="flex">
      <Button
        size="sm"
        onClick={onSubmit}
        disabled={disabled}
        className="h-6 cursor-pointer rounded-r-none px-2 text-xs"
      >
        <IconSend className="mr-1 h-3 w-3" />
        Add
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            disabled={disabled}
            className="h-6 cursor-pointer rounded-l-none border-l border-primary-foreground/20 px-1 text-xs"
          >
            <IconChevronDown className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-[140px]">
          <DropdownMenuItem onClick={onSubmitAndRun} className="cursor-pointer">
            <IconPlayerPlay className="h-3 w-3" />
            Add + Run
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

export function CommentForm({
  initialContent = "",
  onSubmit,
  onCancel,
  onSubmitAndRun,
  isEditing = false,
  autoFocus = true,
}: CommentFormProps) {
  const [content, setContent] = useState(initialContent);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (autoFocus && textareaRef.current) textareaRef.current.focus();
  }, [autoFocus]);

  const handleSubmit = () => {
    const trimmed = content.trim();
    if (trimmed) {
      onSubmit(trimmed);
      setContent("");
    }
  };

  const handleSubmitAndRun = () => {
    const trimmed = content.trim();
    if (trimmed && onSubmitAndRun) {
      onSubmitAndRun(trimmed);
      setContent("");
    }
  };

  const isMac = typeof navigator !== "undefined" && navigator.platform?.includes("Mac");
  const modKey = isMac ? "⌘" : "Ctrl";

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (e.shiftKey && onSubmitAndRun) handleSubmitAndRun();
      else handleSubmit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

  const disabled = !content.trim();

  return (
    <div className="flex flex-col gap-2 rounded-md border border-border bg-card p-2 shadow-md">
      <Textarea
        ref={textareaRef}
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Add a comment..."
        className="min-h-[60px] resize-none text-xs"
        rows={2}
      />
      <div className="flex items-center justify-between gap-2">
        <span className="text-[10px] text-muted-foreground">
          {modKey}+Enter to add{onSubmitAndRun ? `, ${modKey}+Shift+Enter to run` : ""}
        </span>
        <div className="flex gap-1">
          <Button
            size="sm"
            variant="ghost"
            onClick={onCancel}
            className="h-6 cursor-pointer px-2 text-xs"
          >
            <IconX className="mr-1 h-3 w-3" />
            Cancel
          </Button>
          {onSubmitAndRun && !isEditing ? (
            <SplitAddButton
              disabled={disabled}
              onSubmit={handleSubmit}
              onSubmitAndRun={handleSubmitAndRun}
            />
          ) : (
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={disabled}
              className="h-6 cursor-pointer px-2 text-xs"
            >
              <IconSend className="mr-1 h-3 w-3" />
              {isEditing ? "Update" : "Add"}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
