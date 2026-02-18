'use client';

import {
  forwardRef,
  useRef,
  useImperativeHandle,
  type KeyboardEvent,
  type ChangeEvent,
  useCallback,
} from 'react';
import { cn } from '@/lib/utils';

function isSubmitKeyPress(event: KeyboardEvent<HTMLTextAreaElement>, submitKey: 'enter' | 'cmd_enter'): boolean {
  if (submitKey === 'enter') {
    return event.key === 'Enter' && !event.shiftKey && !event.metaKey && !event.ctrlKey;
  }
  return event.key === 'Enter' && (event.metaKey || event.ctrlKey);
}

const MIRROR_STYLES = [
  'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing',
  'textTransform', 'wordSpacing', 'textIndent', 'whiteSpace', 'wordWrap',
  'wordBreak', 'overflowWrap', 'lineHeight', 'padding', 'paddingTop',
  'paddingRight', 'paddingBottom', 'paddingLeft', 'borderWidth', 'boxSizing',
] as const;

function measureCaretRect(textarea: HTMLTextAreaElement, value: string): DOMRect {
  const selectionStart = textarea.selectionStart;
  const computed = window.getComputedStyle(textarea);
  const mirror = document.createElement('div');

  mirror.style.position = 'absolute';
  mirror.style.visibility = 'hidden';
  mirror.style.whiteSpace = 'pre-wrap';
  mirror.style.wordWrap = 'break-word';
  mirror.style.width = `${textarea.clientWidth}px`;
  MIRROR_STYLES.forEach((prop) => { mirror.style[prop as unknown as number] = computed[prop]; });

  document.body.appendChild(mirror);

  mirror.textContent = value.substring(0, selectionStart);
  const marker = document.createElement('span');
  marker.textContent = '\u200B';
  mirror.appendChild(marker);

  const textareaRect = textarea.getBoundingClientRect();
  const markerRect = marker.getBoundingClientRect();
  const mirrorRect = mirror.getBoundingClientRect();
  const scrollTop = textarea.scrollTop;

  document.body.removeChild(mirror);

  return new DOMRect(
    textareaRect.left + (markerRect.left - mirrorRect.left),
    textareaRect.top + (markerRect.top - mirrorRect.top) - scrollTop,
    0,
    parseInt(computed.lineHeight, 10) || parseInt(computed.fontSize, 10) * 1.2
  );
}

export type RichTextInputHandle = {
  focus: () => void;
  blur: () => void;
  getSelectionStart: () => number;
  getSelectionEnd: () => number;
  setSelectionRange: (start: number, end: number) => void;
  getCaretRect: () => DOMRect | null;
  getValue: () => string;
  setValue: (value: string) => void;
  insertText: (text: string, from: number, to: number) => void;
  getTextareaElement: () => HTMLTextAreaElement | null;
};

type RichTextInputProps = {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  onSubmit?: () => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  planModeEnabled?: boolean;
  submitKey?: 'enter' | 'cmd_enter';
  onFocus?: () => void;
  onBlur?: () => void;
};

export const RichTextInput = forwardRef<RichTextInputHandle, RichTextInputProps>(
  function RichTextInput(
    { value, onChange, onKeyDown, onSubmit, placeholder, disabled = false, className, planModeEnabled = false, submitKey = 'cmd_enter', onFocus, onBlur },
    ref
  ) {
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    const getCaretRect = useCallback((): DOMRect | null => {
      const textarea = textareaRef.current;
      if (!textarea) return null;
      return measureCaretRect(textarea, value);
    }, [value]);

    useImperativeHandle(ref, () => ({
      focus: () => textareaRef.current?.focus(),
      blur: () => textareaRef.current?.blur(),
      getSelectionStart: () => textareaRef.current?.selectionStart ?? 0,
      getSelectionEnd: () => textareaRef.current?.selectionEnd ?? 0,
      setSelectionRange: (start: number, end: number) => { textareaRef.current?.setSelectionRange(start, end); },
      getCaretRect,
      getValue: () => textareaRef.current?.value ?? '',
      setValue: (newValue: string) => onChange(newValue),
      insertText: (text: string, from: number, to: number) => {
        const newValue = value.substring(0, from) + text + value.substring(to);
        onChange(newValue);
        requestAnimationFrame(() => { textareaRef.current?.setSelectionRange(from + text.length, from + text.length); });
      },
      getTextareaElement: () => textareaRef.current,
    }), [getCaretRect, onChange, value]);

    const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => { onChange(event.target.value); };

    const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
      if (onKeyDown) {
        onKeyDown(event);
        if (event.defaultPrevented) return;
      }
      const isSubmitKey = isSubmitKeyPress(event, submitKey);
      if (disabled) {
        if (isSubmitKey) event.preventDefault();
        return;
      }
      if (isSubmitKey) { event.preventDefault(); onSubmit?.(); }
    };

    return (
      <textarea
        ref={textareaRef}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onFocus={onFocus}
        onBlur={onBlur}
        placeholder={placeholder}
        disabled={disabled}
        className={cn(
          'w-full h-full resize-none bg-transparent px-2 py-2 overflow-y-auto',
          'text-sm leading-relaxed',
          'placeholder:text-muted-foreground',
          'focus:outline-none',
          'disabled:cursor-not-allowed disabled:opacity-50',
          planModeEnabled && 'border-primary/40',
          className
        )}
      />
    );
  }
);
