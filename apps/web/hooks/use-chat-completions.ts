import { useMemo } from 'react';
import { EditorView } from '@codemirror/view';
import type { CompletionContext } from '@codemirror/autocomplete';
import { useCustomPrompts } from '@/hooks/use-custom-prompts';

const truncatePreview = (text: string, max = 140) => {
  if (text.length <= max) return text;
  return `${text.slice(0, max)}…`;
};

export function useChatCompletions() {
  const { prompts } = useCustomPrompts();

  const promptOptions = useMemo(() => {
    return prompts.map((prompt) => ({
      id: prompt.id,
      name: prompt.name,
      content: prompt.content,
    }));
  }, [prompts]);

  const completionSource = useMemo(() => {
    return (context: CompletionContext) => {
      // Slash command completions
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

      // @prompt mention completions
      const mentionMatch = context.matchBefore(/(^|\s)@[\w-]*/);
      if (mentionMatch) {
        const from = mentionMatch.from + (mentionMatch.text.startsWith('@') ? 0 : 1);
        const trimmed = mentionMatch.text.trim();
        const query = trimmed.startsWith('@') ? trimmed.slice(1).toLowerCase() : trimmed.toLowerCase();
        const promptMatches = promptOptions
          .filter((prompt) => prompt.name.toLowerCase().startsWith(query))
          .map((prompt) => ({
            label: `@${prompt.name}`,
            type: 'prompt',
            detail: 'Prompt',
            info: truncatePreview(prompt.content),
            apply: (view: EditorView, _completion: unknown, applyFrom: number, applyTo: number) => {
              const insertText = `${prompt.content}\n`;
              const cursorPos = applyFrom + insertText.length;
              view.dispatch({
                changes: { from: applyFrom, to: applyTo, insert: insertText },
                selection: { anchor: cursorPos },
              });
              view.dispatch({ effects: EditorView.scrollIntoView(cursorPos) });
            },
          }));
        return {
          from,
          // Future: add file path completions alongside prompts using additional providers.
          options: [...promptMatches],
        };
      }

      return null;
    };
  }, [promptOptions]);

  return completionSource;
}
