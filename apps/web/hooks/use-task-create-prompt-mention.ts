"use client";

import { useLayoutEffect, useMemo, useRef } from "react";
import { useInlineMention } from "@/hooks/use-inline-mention";
import { measureCaretRect } from "@/lib/utils/caret-position";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";

type Params = {
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  value: string;
  onChange: (newValue: string) => void;
};

/**
 * Wires the `useInlineMention` autocomplete hook to a plain `<textarea>` so
 * the task-create prompt input can offer `@`-mention selection of saved
 * custom prompts. Inlines the prompt's content text at the trigger position
 * (no context chips, no backend changes).
 */
export function useTaskCreatePromptMention({ textareaRef, value, onChange }: Params) {
  const adapter = useMemo<RichTextInputHandle>(
    () => ({
      focus: () => textareaRef.current?.focus(),
      blur: () => textareaRef.current?.blur(),
      getSelectionStart: () => textareaRef.current?.selectionStart ?? 0,
      getSelectionEnd: () => textareaRef.current?.selectionEnd ?? 0,
      setSelectionRange: (start: number, end: number) => {
        textareaRef.current?.setSelectionRange(start, end);
      },
      getCaretRect: () => {
        const ta = textareaRef.current;
        return ta ? measureCaretRect(ta, ta.value) : null;
      },
      getValue: () => textareaRef.current?.value ?? "",
      setValue: (v: string) => onChange(v),
      insertText: (text: string, from: number, to: number) => {
        const current = textareaRef.current?.value ?? "";
        const newValue = current.substring(0, from) + text + current.substring(to);
        onChange(newValue);
        requestAnimationFrame(() => {
          textareaRef.current?.setSelectionRange(from + text.length, from + text.length);
        });
      },
      getTextareaElement: () => textareaRef.current,
    }),
    [textareaRef, onChange],
  );

  const adapterRef = useRef<RichTextInputHandle | null>(adapter);
  useLayoutEffect(() => {
    adapterRef.current = adapter;
  }, [adapter]);

  return useInlineMention({
    inputRef: adapterRef,
    value,
    onChange,
    promptInsertMode: "inline",
  });
}
