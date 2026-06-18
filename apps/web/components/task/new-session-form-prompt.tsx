"use client";

import type { RefObject } from "react";
import { Textarea } from "@kandev/ui/textarea";
import { IconLoader2 } from "@tabler/icons-react";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { ContextZone } from "./chat/context-items/context-zone";
import { AttachButton } from "./session-dialog-shared";
import type { ContextItem } from "@/lib/types/context";

type SessionPromptFieldProps = {
  promptRef: RefObject<HTMLTextAreaElement | null>;
  contextItems: ContextItem[];
  isBusy: boolean;
  isDragging: boolean;
  isSummarizing: boolean;
  hasPrompt: boolean;
  hasProfiles: boolean;
  isUtilityConfigured: boolean;
  isEnhancingPrompt: boolean;
  fileInputRef: RefObject<HTMLInputElement | null>;
  onPromptInput: () => void;
  onPaste: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void;
  onSubmit: (e: React.FormEvent) => void;
  onAttachClick: () => void;
  onEnhancePrompt: () => void;
  onDragOver: (e: React.DragEvent) => void;
  onDragLeave: (e: React.DragEvent) => void;
  onDrop: (e: React.DragEvent) => void;
  onFileInputChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
};

export function SessionPromptField({
  promptRef,
  contextItems,
  isBusy,
  isDragging,
  isSummarizing,
  hasPrompt,
  hasProfiles,
  isUtilityConfigured,
  isEnhancingPrompt,
  fileInputRef,
  onPromptInput,
  onPaste,
  onSubmit,
  onAttachClick,
  onEnhancePrompt,
  onDragOver,
  onDragLeave,
  onDrop,
  onFileInputChange,
}: SessionPromptFieldProps) {
  return (
    <div
      className="relative min-w-0 max-w-full"
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <div className="min-w-0 max-w-full rounded-md border border-input bg-transparent focus-within:ring-2 focus-within:ring-ring/30">
        <ContextZone items={contextItems} />
        <Textarea
          ref={promptRef}
          placeholder="What should the agent work on?"
          className="min-w-0 max-w-full field-sizing-fixed wrap-anywhere border-0 focus-visible:ring-0 focus-visible:ring-offset-0 min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]"
          autoFocus
          disabled={isBusy}
          onInput={onPromptInput}
          onPaste={onPaste}
          onKeyDown={(e) => {
            if (
              e.key === "Enter" &&
              (e.metaKey || e.ctrlKey) &&
              !isBusy &&
              hasPrompt &&
              hasProfiles
            ) {
              e.preventDefault();
              onSubmit(e);
            }
          }}
        />
        <div className="flex items-center px-1 pb-1">
          <AttachButton onClick={onAttachClick} disabled={isBusy} />
          <EnhancePromptButton
            onClick={onEnhancePrompt}
            isLoading={isEnhancingPrompt}
            isConfigured={isUtilityConfigured}
          />
        </div>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={onFileInputChange}
          tabIndex={-1}
        />
      </div>
      {isDragging && (
        <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
          <span className="text-sm text-primary font-medium">Drop files here</span>
        </div>
      )}
      {isSummarizing && (
        <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/80">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            <span>Generating summary...</span>
          </div>
        </div>
      )}
    </div>
  );
}
