"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { IconSend, IconX } from "@tabler/icons-react";

interface CommentFormProps {
  /** Initial content for editing */
  initialContent?: string;
  /** Callback when comment is submitted */
  onSubmit: (content: string) => void;
  /** Callback when form is cancelled */
  onCancel: () => void;
  /** Whether the form is in editing mode */
  isEditing?: boolean;
  /** Auto focus the textarea */
  autoFocus?: boolean;
}

export function CommentForm({
  initialContent = "",
  onSubmit,
  onCancel,
  isEditing = false,
  autoFocus = true,
}: CommentFormProps) {
  const [content, setContent] = useState(initialContent);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (autoFocus && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [autoFocus]);

  const handleSubmit = () => {
    const trimmed = content.trim();
    if (trimmed) {
      onSubmit(trimmed);
      setContent("");
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      handleSubmit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

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
          {typeof navigator !== "undefined" && navigator.platform?.includes("Mac") ? "⌘" : "Ctrl"}
          +Enter to submit
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
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={!content.trim()}
            className="h-6 cursor-pointer px-2 text-xs"
          >
            <IconSend className="mr-1 h-3 w-3" />
            {isEditing ? "Update" : "Add"}
          </Button>
        </div>
      </div>
    </div>
  );
}
