'use client';

import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view';
import { autocompletion, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { Prec } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { cn } from '@/lib/utils';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { matchesShortcut, shortcutToCodeMirrorKeybinding } from '@/lib/keyboard/utils';
import { chatEditorTheme } from '@/lib/editor/chat-editor-theme';
import { useChatCompletions } from '@/hooks/use-chat-completions';

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
  const completionSource = useChatCompletions();

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
    chatEditorTheme,
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
        'task-chat-editor rounded-md border border-input bg-muted/40 focus-within:border-ring focus-within:ring-[2px] focus-within:ring-ring/30 overflow-hidden',
        planModeEnabled &&
        'border-dashed border-primary/60 !bg-primary/10 shadow-[inset_0_0_0_1px_rgba(59,130,246,0.35)]',
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
        className="text-xs"
      />
    </div>
  );
}
