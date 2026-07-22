import type { languages, editor, IRange } from "monaco-editor";
import type { PromptReference } from "@/lib/prompts/expand-prompt-references";

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

const MENTION_NAME_PATTERN = /[\w-]*$/;

function isMentionStartColumn(line: string, atColumn: number): boolean {
  if (atColumn === 1) return true;
  const charBefore = line[atColumn - 2];
  return charBefore === " " || charBefore === "\t" || charBefore === "\r";
}

function previewContent(content: string): string {
  const firstLine = content.split("\n")[0]?.trim() ?? "";
  return firstLine.length > 80 ? `${firstLine.slice(0, 80)}…` : firstLine;
}

/**
 * Creates a Monaco CompletionItemProvider that suggests saved custom prompts
 * as @name mentions. Triggers on `@`, only at a valid mention start (start of
 * line or preceded by whitespace), matching the same rule as
 * `matchPromptMention`/`isPromptReferenceStart`.
 */
export function createPromptMentionCompletionProvider(
  monaco: typeof import("monaco-editor"),
  prompts: PromptReference[],
): languages.CompletionItemProvider {
  return {
    triggerCharacters: ["@"],
    provideCompletionItems(
      model: editor.ITextModel,
      position: { lineNumber: number; column: number },
    ): languages.ProviderResult<languages.CompletionList> {
      const line = model.getLineContent(position.lineNumber);
      const textBefore = line.substring(0, position.column - 1);

      const nameMatch = textBefore.match(MENTION_NAME_PATTERN);
      const namePrefix = nameMatch ? nameMatch[0] : "";
      const atColumn = position.column - namePrefix.length - 1;

      if (atColumn < 1 || line[atColumn - 1] !== "@" || !isMentionStartColumn(line, atColumn)) {
        return { suggestions: [] };
      }

      const range: IRange = {
        startLineNumber: position.lineNumber,
        startColumn: atColumn + 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      };

      const suggestions: languages.CompletionItem[] = prompts.map((prompt, i) => ({
        label: `@${prompt.name}`,
        kind: monaco.languages.CompletionItemKind.Reference,
        detail: previewContent(prompt.content),
        documentation: previewContent(prompt.content),
        insertText: prompt.name,
        range,
        sortText: String(i).padStart(3, "0"),
      }));

      return { suggestions };
    },
  };
}
