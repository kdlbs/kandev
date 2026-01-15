'use client';

import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view';
import { autocompletion, closeBrackets, closeBracketsKeymap, CompletionContext } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { Prec } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { cn } from '@/lib/utils';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { matchesShortcut, shortcutToCodeMirrorKeybinding } from '@/lib/keyboard/utils';

type TaskChatInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  planModeEnabled?: boolean;
};

export function TaskChatInput({
  value,
  onChange,
  onSubmit,
  placeholder,
  disabled,
  className,
  planModeEnabled,
}: TaskChatInputProps) {
  const submitKey = shortcutToCodeMirrorKeybinding(SHORTCUTS.SUBMIT);

  const completionSource = (context: CompletionContext) => {
    const slashMatch = context.matchBefore(/(^|\s)\/[\w-]*/);
    if (slashMatch) {
      const from = slashMatch.from + (slashMatch.text.startsWith('/') ? 0 : 1);
      return {
        from,
        options: [
          { label: '/plan', type: 'slash', info: 'Start a planning response' },
          { label: '/todo', type: 'slash', info: 'Add a todo list' },
          { label: '/review', type: 'slash', info: 'Request a review' },
          { label: '/summarize', type: 'slash', info: 'Summarize the task' },
        ],
      };
    }

    const mentionMatch = context.matchBefore(/(^|\s)@[\w-]*/);
    if (mentionMatch) {
      const from = mentionMatch.from + (mentionMatch.text.startsWith('@') ? 0 : 1);
      return {
        from,
        options: [
          { label: '@agent', type: 'mention', info: 'Notify the agent' },
          { label: '@reviewer', type: 'mention', info: 'Tag a reviewer' },
          { label: '@designer', type: 'mention', info: 'Tag design feedback' },
        ],
      };
    }

    return null;
  };

  const editorTheme = EditorView.theme({
    '&': {
      color: 'var(--foreground)',
      fontSize: '0.875rem',
      backgroundColor: 'transparent',
    },
    '.cm-editor': {
      borderRadius: 'inherit',
      background: 'transparent',
      outline: 'none',
      boxShadow: 'none',
      overflow: 'hidden',
    },
    '.cm-editor.cm-focused': {
      outline: 'none',
      boxShadow: 'none',
    },
    '.cm-scroller': {
      fontFamily: 'var(--font-sans)',
      borderRadius: 'inherit',
      background: 'transparent',
      backgroundColor: 'transparent',
      overflow: 'auto',
    },
    '.cm-content': {
      padding: '0.5rem',
      fontFamily: 'var(--font-sans)',
      color: 'var(--foreground)',
      background: 'transparent',
    },
    '.cm-line': {
      color: 'var(--foreground)',
    },
    '.cm-placeholder': {
      color: 'var(--muted-foreground)',
      opacity: '0.8',
    },
    '.cm-tooltip-autocomplete, .cm-tooltip-autocomplete li, .cm-completionDetail': {
      fontSize: '0.875rem',
    },
    '.cm-cursor': {
      borderLeftColor: 'var(--foreground)',
    },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--primary)',
      opacity: '0.7',
    },
    '.cm-selectionMatch': {
      backgroundColor: 'var(--primary)',
      opacity: '0.6',
    },
    '.cm-selectionLayer .cm-selectionBackground': {
      backgroundColor: 'var(--primary)',
      opacity: '0.7',
    },
  });

  const handleSubmitKey = () => {
    onSubmit();
    return true;
  };

  const submitKeymap = Prec.highest(
    keymap.of([
      { key: submitKey, run: handleSubmitKey, preventDefault: true },
      { key: 'Mod-Enter', run: handleSubmitKey, preventDefault: true },
    ])
  );

  const extensions: Extension[] = [
    EditorView.lineWrapping,
    editorTheme,
    EditorView.domEventHandlers({
      keydown: (event) => {
        if (matchesShortcut(event, SHORTCUTS.SUBMIT)) {
          event.preventDefault();
          event.stopPropagation();
          onSubmit();
          return true;
        }
        return false;
      },
    }),
    history(),
    closeBrackets(),
    markdown(),
    cmPlaceholder(placeholder ?? ''),
    autocompletion({ override: [completionSource], aboveCursor: true }),
    submitKeymap,
    keymap.of([...defaultKeymap, ...historyKeymap, ...closeBracketsKeymap]),
  ];

  return (
    <div
      className={cn(
        'task-chat-editor rounded-md border border-input bg-input/20 dark:bg-input/30 focus-within:border-ring focus-within:ring-[2px] focus-within:ring-ring/30 overflow-hidden',
        planModeEnabled &&
          'border-dashed border-primary/60 !bg-primary/20 dark:!bg-primary/20 shadow-[inset_0_0_0_1px_rgba(59,130,246,0.35)]',
        className
      )}
    >
      <CodeMirror
        value={value}
        onChange={onChange}
        editable={!disabled}
        minHeight="90px"
        maxHeight="200px"
        extensions={extensions}
        basicSetup={{
          lineNumbers: false,
          foldGutter: false,
          highlightActiveLine: false,
          highlightSelectionMatches: true,
          autocompletion: false,
        }}
        className="text-sm"
      />
    </div>
  );
}
