'use client';

import {
  forwardRef,
  useRef,
  useImperativeHandle,
  type KeyboardEvent,
  type ChangeEvent,
  useCallback,
  useLayoutEffect,
} from 'react';
import { cn } from '@/lib/utils';

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
    {
      value,
      onChange,
      onKeyDown,
      onSubmit,
      placeholder,
      disabled = false,
      className,
      planModeEnabled = false,
      submitKey = 'cmd_enter',
      onFocus,
      onBlur,
    },
    ref
  ) {
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    // Auto-resize textarea based on content
    useLayoutEffect(() => {
      const textarea = textareaRef.current;
      if (!textarea) return;

      // Reset height to auto to get the correct scrollHeight
      textarea.style.height = 'auto';

      // Calculate new height (clamped to min/max)
      const minHeight = 88;
      const maxHeight = Math.min(window.innerHeight * 0.4, 300);
      const newHeight = Math.min(Math.max(textarea.scrollHeight, minHeight), maxHeight);

      textarea.style.height = `${newHeight}px`;
    }, [value]);

    const getCaretRect = useCallback((): DOMRect | null => {
      const textarea = textareaRef.current;
      if (!textarea) return null;

      const selectionStart = textarea.selectionStart;

      // Create a mirror div to measure caret position
      const mirror = document.createElement('div');
      const computed = window.getComputedStyle(textarea);

      // Copy styles that affect text layout
      const stylesToCopy = [
        'fontFamily',
        'fontSize',
        'fontWeight',
        'fontStyle',
        'letterSpacing',
        'textTransform',
        'wordSpacing',
        'textIndent',
        'whiteSpace',
        'wordWrap',
        'wordBreak',
        'overflowWrap',
        'lineHeight',
        'padding',
        'paddingTop',
        'paddingRight',
        'paddingBottom',
        'paddingLeft',
        'borderWidth',
        'boxSizing',
      ] as const;

      mirror.style.position = 'absolute';
      mirror.style.visibility = 'hidden';
      mirror.style.whiteSpace = 'pre-wrap';
      mirror.style.wordWrap = 'break-word';
      mirror.style.width = `${textarea.clientWidth}px`;

      stylesToCopy.forEach((prop) => {
        mirror.style[prop as unknown as number] = computed[prop];
      });

      document.body.appendChild(mirror);

      // Get text before cursor and add a span for measuring
      const textBeforeCursor = value.substring(0, selectionStart);
      mirror.textContent = textBeforeCursor;

      // Add a marker span at the cursor position
      const marker = document.createElement('span');
      marker.textContent = '\u200B'; // Zero-width space
      mirror.appendChild(marker);

      // Get textarea position and marker position
      const textareaRect = textarea.getBoundingClientRect();
      const markerRect = marker.getBoundingClientRect();
      const mirrorRect = mirror.getBoundingClientRect();

      // Calculate position relative to viewport
      const relativeTop = markerRect.top - mirrorRect.top;
      const relativeLeft = markerRect.left - mirrorRect.left;

      // Account for scroll position in textarea
      const scrollTop = textarea.scrollTop;

      document.body.removeChild(mirror);

      return new DOMRect(
        textareaRect.left + relativeLeft,
        textareaRect.top + relativeTop - scrollTop,
        0,
        parseInt(computed.lineHeight, 10) || parseInt(computed.fontSize, 10) * 1.2
      );
    }, [value]);

    useImperativeHandle(
      ref,
      () => ({
        focus: () => textareaRef.current?.focus(),
        blur: () => textareaRef.current?.blur(),
        getSelectionStart: () => textareaRef.current?.selectionStart ?? 0,
        getSelectionEnd: () => textareaRef.current?.selectionEnd ?? 0,
        setSelectionRange: (start: number, end: number) => {
          textareaRef.current?.setSelectionRange(start, end);
        },
        getCaretRect,
        getValue: () => textareaRef.current?.value ?? '',
        setValue: (newValue: string) => onChange(newValue),
        insertText: (text: string, from: number, to: number) => {
          const newValue = value.substring(0, from) + text + value.substring(to);
          onChange(newValue);
          // Set cursor position after the inserted text
          requestAnimationFrame(() => {
            textareaRef.current?.setSelectionRange(from + text.length, from + text.length);
          });
        },
        getTextareaElement: () => textareaRef.current,
      }),
      [getCaretRect, onChange, value]
    );

    const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
      onChange(event.target.value);
    };

    const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
      // Let parent handle key events first (for menu navigation)
      if (onKeyDown) {
        onKeyDown(event);
        if (event.defaultPrevented) return;
      }

      if (submitKey === 'enter') {
        // Submit on Enter (unless Shift for newline)
        if (event.key === 'Enter' && !event.shiftKey && !event.metaKey && !event.ctrlKey) {
          event.preventDefault();
          if (!disabled) {
            onSubmit?.();
          }
        }
      } else {
        // Submit on Cmd/Ctrl+Enter (current behavior)
        if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          if (!disabled) {
            onSubmit?.();
          }
        }
      }
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
          'w-full resize-none bg-transparent px-3 py-3',
          'text-sm leading-relaxed',
          'placeholder:text-muted-foreground',
          'focus:outline-none',
          'disabled:cursor-not-allowed disabled:opacity-50',
          planModeEnabled && 'border-primary/40',
          className
        )}
        style={{
          minHeight: '88px',
          maxHeight: 'min(40vh, 300px)',
        }}
      />
    );
  }
);
