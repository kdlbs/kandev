"use client";

import { useCallback, type KeyboardEvent } from "react";
import { IconFile, IconRobot, IconSend } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import {
  handleSuggestionKey,
  suggestionDescription,
  suggestionLabel,
  usePassthroughComposerController,
  type AgentCommand,
  type SuggestionItem,
} from "./passthrough-composer-state";
import type { PassthroughSuggestionState } from "./passthrough-composer-helpers";

type PassthroughComposerProps = {
  onSubmit: (content: string) => Promise<void>;
  onCancel?: () => void;
  autoFocus?: boolean;
  placeholder?: string;
  /** Optional content rendered above the textarea — e.g. a pending-comments preview. */
  header?: React.ReactNode;
  sessionId?: string | null;
  availableCommands?: AgentCommand[];
  focusShortcut?: KeyboardShortcut;
};

function usePassthroughKeyDown({
  showSuggestions,
  suggestionItems,
  selectedIndex,
  setSelectedIndex,
  insertSelection,
  submit,
  suggestion,
  setSuggestion,
  onCancel,
}: {
  showSuggestions: boolean;
  suggestionItems: SuggestionItem[];
  selectedIndex: number;
  setSelectedIndex: (v: number | ((i: number) => number)) => void;
  insertSelection: (item: SuggestionItem) => void;
  submit: () => Promise<void>;
  suggestion: PassthroughSuggestionState;
  setSuggestion: (suggestion: PassthroughSuggestionState) => void;
  onCancel?: () => void;
}) {
  return useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (
        handleSuggestionKey(e, {
          showSuggestions,
          suggestionItems,
          selectedIndex,
          setSelectedIndex,
          insertSelection,
        })
      ) {
        return;
      }
      if (e.key === "Enter" && !e.shiftKey) {
        if (e.nativeEvent.isComposing) return;
        e.preventDefault();
        void submit();
        return;
      }
      if (e.key === "Escape" && onCancel) {
        e.preventDefault();
        if (suggestion) {
          setSuggestion(null);
          return;
        }
        onCancel();
      }
    },
    [
      insertSelection,
      onCancel,
      selectedIndex,
      setSelectedIndex,
      setSuggestion,
      showSuggestions,
      submit,
      suggestion,
      suggestionItems,
    ],
  );
}

/**
 * PassthroughComposer is the kandev-controlled compose box rendered alongside
 * the PTY in passthrough mode. Enter submits; Shift+Enter inserts a newline.
 */
export function PassthroughComposer({
  onSubmit,
  onCancel,
  autoFocus = false,
  placeholder = "Message the CLI agent (Enter to send, Shift+Enter for newline)",
  header,
  sessionId,
  availableCommands,
  focusShortcut,
}: PassthroughComposerProps) {
  const {
    value,
    isSending,
    canSubmit,
    textareaRef,
    suggestion,
    suggestionItems,
    showSuggestions,
    selectedIndex,
    setSelectedIndex,
    updateValue,
    submit,
    insertSelection,
    handleDrop,
    setSuggestion,
  } = usePassthroughComposerController({
    onSubmit,
    autoFocus,
    sessionId,
    availableCommands,
    focusShortcut,
  });

  const handleKeyDown = usePassthroughKeyDown({
    showSuggestions,
    suggestionItems,
    selectedIndex,
    setSelectedIndex,
    insertSelection,
    submit,
    suggestion,
    setSuggestion,
    onCancel,
  });

  return (
    <div
      className="flex flex-col flex-shrink-0 border-t bg-card"
      data-testid="passthrough-composer"
    >
      {header}
      <div className="relative flex items-end gap-2 px-2 py-2">
        <Textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => updateValue(e.target.value)}
          onKeyDown={handleKeyDown}
          onDrop={handleDrop}
          onDragOver={(e) => e.preventDefault()}
          placeholder={placeholder}
          rows={1}
          disabled={isSending}
          className="min-h-9 max-h-32 flex-1 resize-none overflow-y-auto"
          data-testid="passthrough-composer-textarea"
        />
        <PassthroughSuggestionMenu
          open={showSuggestions}
          suggestion={suggestion}
          items={suggestionItems}
          selectedIndex={selectedIndex}
          setSelectedIndex={setSelectedIndex}
          onSelect={insertSelection}
        />
        <Button
          type="button"
          size="sm"
          variant="default"
          onClick={() => void submit()}
          disabled={!canSubmit}
          className="cursor-pointer h-9 shrink-0"
          data-testid="passthrough-composer-submit"
          aria-label="Send message to CLI agent"
        >
          <IconSend className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function PassthroughSuggestionMenu({
  open,
  suggestion,
  items,
  selectedIndex,
  setSelectedIndex,
  onSelect,
}: {
  open: boolean;
  suggestion: PassthroughSuggestionState;
  items: SuggestionItem[];
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
  onSelect: (item: SuggestionItem) => void;
}) {
  if (!open || !suggestion) return null;
  return (
    <div
      className="absolute bottom-14 left-2 z-20 w-80 max-w-[calc(100vw-1rem)] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md"
      data-testid="passthrough-composer-suggestions"
    >
      <div className="border-b px-3 py-1 text-xs font-medium text-muted-foreground">
        {suggestion.kind === "command" ? "Commands" : "Files"}
      </div>
      {items.map((item, index) => (
        <SuggestionButton
          key={suggestionLabel(item)}
          item={item}
          kind={suggestion.kind}
          selected={selectedIndex === index}
          onSelect={onSelect}
          onHover={() => setSelectedIndex(index)}
        />
      ))}
    </div>
  );
}

function SuggestionButton({
  item,
  kind,
  selected,
  onSelect,
  onHover,
}: {
  item: SuggestionItem;
  kind: "command" | "file";
  selected: boolean;
  onSelect: (item: SuggestionItem) => void;
  onHover: () => void;
}) {
  return (
    <button
      type="button"
      className={`flex w-full items-start gap-2 px-3 py-2 text-left text-sm ${
        selected ? "bg-accent" : ""
      }`}
      onMouseEnter={onHover}
      onClick={() => onSelect(item)}
    >
      {kind === "command" ? (
        <IconRobot className="mt-0.5 h-4 w-4 shrink-0" />
      ) : (
        <IconFile className="mt-0.5 h-4 w-4 shrink-0" />
      )}
      <span className="min-w-0">
        <span className="block truncate">{suggestionLabel(item)}</span>
        {suggestionDescription(item) && (
          <span className="block truncate text-xs text-muted-foreground">
            {suggestionDescription(item)}
          </span>
        )}
      </span>
    </button>
  );
}
