"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { IconSend } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";

// Cap matches the `max-h-32` Tailwind class on the textarea below — 128 px.
const COMPOSER_MAX_HEIGHT_PX = 128;

/**
 * PassthroughComposer is the kandev-controlled compose box rendered alongside
 * the PTY in passthrough mode. Submitting forwards the typed text via the
 * onSubmit prop (which posts `message.add` over WS); the backend's
 * `Executor.Prompt` routes passthrough sessions to PTY stdin so the CLI agent
 * actually receives it. Enter submits; Shift+Enter inserts a newline.
 */
export function PassthroughComposer({
  onSubmit,
  onCancel,
  autoFocus = false,
  placeholder = "Message the CLI agent (Enter to send, Shift+Enter for newline)",
  header,
}: {
  onSubmit: (content: string) => Promise<void>;
  onCancel?: () => void;
  autoFocus?: boolean;
  placeholder?: string;
  /** Optional content rendered above the textarea — e.g. a pending-comments preview. */
  header?: React.ReactNode;
}) {
  const [value, setValue] = useState("");
  const [isSending, setIsSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0 && !isSending;

  // Auto-grow the textarea with content (Shift+Enter newlines, long pastes)
  // up to the max-h-32 cap. Done in JS so it works in browsers without
  // CSS field-sizing support.
  useLayoutEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_MAX_HEIGHT_PX)}px`;
  }, [value]);

  useEffect(() => {
    if (autoFocus) {
      textareaRef.current?.focus();
    }
  }, [autoFocus]);

  const submit = useCallback(async () => {
    if (!canSubmit) return;
    setIsSending(true);
    try {
      await onSubmit(trimmed);
      setValue(""); // only clear on success — preserve text so user can retry on send failure
    } catch {
      // onSubmit already surfaced the error via toast; the user's typed value
      // stays in the textarea intentionally so they can retry without retyping.
    } finally {
      setIsSending(false);
    }
  }, [canSubmit, onSubmit, trimmed]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        if (e.nativeEvent.isComposing) return;
        e.preventDefault();
        void submit();
        return;
      }
      if (e.key === "Escape" && onCancel) {
        e.preventDefault();
        onCancel();
      }
    },
    [submit, onCancel],
  );

  return (
    <div
      className="flex flex-col flex-shrink-0 border-t bg-card"
      data-testid="passthrough-composer"
    >
      {header}
      <div className="flex items-end gap-2 px-2 py-2">
        <Textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          rows={1}
          disabled={isSending}
          className="min-h-9 max-h-32 flex-1 resize-none overflow-y-auto"
          data-testid="passthrough-composer-textarea"
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
