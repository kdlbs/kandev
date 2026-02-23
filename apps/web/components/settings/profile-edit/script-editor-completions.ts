import type { languages, editor, IRange } from "monaco-editor";

export type ScriptPlaceholder = {
  key: string;
  description: string;
  example: string;
  executor_types: string[];
};

/**
 * Creates a Monaco CompletionItemProvider that suggests {{placeholder}} values.
 * Triggers on `{` and filters by executor type.
 */
export function createPlaceholderCompletionProvider(
  monaco: typeof import("monaco-editor"),
  placeholders: ScriptPlaceholder[],
  executorType?: string,
): languages.CompletionItemProvider {
  return {
    triggerCharacters: ["{"],
    provideCompletionItems(
      model: editor.ITextModel,
      position: { lineNumber: number; column: number },
    ): languages.ProviderResult<languages.CompletionList> {
      const line = model.getLineContent(position.lineNumber);
      const textBefore = line.substring(0, position.column - 1);

      // Only trigger after `{{`
      if (!textBefore.endsWith("{{") && !textBefore.match(/\{\{[\w.]*$/)) {
        return { suggestions: [] };
      }

      // Find the range to replace (from {{ to cursor)
      const match = textBefore.match(/\{\{([\w.]*)$/);
      const startCol = match ? position.column - match[1].length : position.column;

      const range: IRange = {
        startLineNumber: position.lineNumber,
        startColumn: startCol,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      };

      const filtered = executorType
        ? placeholders.filter(
            (p) => p.executor_types.length === 0 || p.executor_types.includes(executorType),
          )
        : placeholders;

      const suggestions: languages.CompletionItem[] = filtered.map((p, i) => ({
        label: {
          label: `{{${p.key}}}`,
          description: p.description,
        },
        kind: monaco.languages.CompletionItemKind.Variable,
        detail: p.description,
        documentation: p.example ? `Example: ${p.example}` : undefined,
        insertText: `${p.key}}}`,
        range,
        sortText: String(i).padStart(3, "0"),
      }));

      return { suggestions };
    },
  };
}
