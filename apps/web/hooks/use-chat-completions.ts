import { useMemo, useRef, useCallback } from 'react';
import { EditorView } from '@codemirror/view';
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { useCustomPrompts } from '@/hooks/domains/settings/use-custom-prompts';
import type { CustomPrompt } from '@/lib/types/http';
import { getWebSocketClient } from '@/lib/ws/connection';
import { searchWorkspaceFiles } from '@/lib/ws/workspace-files';

const truncatePreview = (text: string, max = 140) => {
  if (text.length <= max) return text;
  return `${text.slice(0, max)}…`;
};

// Debounce delay for file search (ms)
const FILE_SEARCH_DEBOUNCE = 300;

export function useChatCompletions(sessionId?: string | null) {
  const { prompts } = useCustomPrompts();
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSearchRef = useRef<{ query: string; results: string[] }>({ query: '', results: [] });

  const promptOptions = useMemo(() => {
    return prompts.map((prompt: CustomPrompt) => ({
      id: prompt.id,
      name: prompt.name,
      content: prompt.content,
    }));
  }, [prompts]);

  // File search function with caching
  const searchFiles = useCallback(async (query: string): Promise<string[]> => {
    if (!sessionId || !query) return [];

    // Return cached results if query matches
    if (lastSearchRef.current.query === query) {
      return lastSearchRef.current.results;
    }

    try {
      const client = getWebSocketClient();
      if (!client) return [];

      const response = await searchWorkspaceFiles(client, sessionId, query, 20);
      const results = response.files || [];

      // Cache results
      lastSearchRef.current = { query, results };
      return results;
    } catch (error) {
      console.error('File search failed:', error);
      return [];
    }
  }, [sessionId]);

  const completionSource = useMemo(() => {
    return async (context: CompletionContext): Promise<CompletionResult | null> => {
      // Slash command completions
      const slashMatch = context.matchBefore(/(^|\s)\/[\w-]*/);
      if (slashMatch) {
        const from = slashMatch.from + (slashMatch.text.startsWith('/') ? 0 : 1);
        return {
          from,
          options: [
            { label: '/plan', type: 'keyword', info: 'Start a planning response' },
            { label: '/todo', type: 'keyword', info: 'Add a todo list' },
            { label: '/review', type: 'keyword', info: 'Request a review' },
            { label: '/summarize', type: 'keyword', info: 'Summarize the task' },
          ],
        };
      }

      // @ mention completions (prompts and files)
      const mentionMatch = context.matchBefore(/(^|\s)@[\w.\/\-_]*/);
      if (mentionMatch) {
        const from = mentionMatch.from + (mentionMatch.text.startsWith('@') ? 0 : 1);
        const trimmed = mentionMatch.text.trim();
        const query = trimmed.startsWith('@') ? trimmed.slice(1).toLowerCase() : trimmed.toLowerCase();

        // Prompt completions (existing behavior)
        const promptMatches = promptOptions
          .filter((prompt: { id: string; name: string; content: string }) =>
            prompt.name.toLowerCase().startsWith(query)
          )
          .map((prompt: { id: string; name: string; content: string }) => ({
            label: `@${prompt.name}`,
            type: 'text',
            detail: 'Prompt',
            info: truncatePreview(prompt.content),
            boost: 10, // Prompts have higher priority
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

        // File completions (only if sessionId is available and query has content)
        let fileMatches: Array<{
          label: string;
          type: string;
          detail: string;
          info: string;
          boost: number;
        }> = [];

        if (sessionId && query.length > 0) {
          // Cancel previous search timeout
          if (searchTimeoutRef.current) {
            clearTimeout(searchTimeoutRef.current);
          }

          // Debounced file search
          const files = await new Promise<string[]>((resolve) => {
            searchTimeoutRef.current = setTimeout(async () => {
              const results = await searchFiles(query);
              resolve(results);
            }, FILE_SEARCH_DEBOUNCE);
          });

          fileMatches = files.map((filePath) => ({
            label: `@${filePath}`,
            type: 'variable',
            detail: 'File',
            info: filePath,
            boost: 5, // Files have lower priority than prompts
          }));
        }

        return {
          from,
          options: [...promptMatches, ...fileMatches],
        };
      }

      return null;
    };
  }, [promptOptions, sessionId, searchFiles]);

  return completionSource;
}
