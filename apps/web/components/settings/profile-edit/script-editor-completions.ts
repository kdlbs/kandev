import type { languages, editor, IRange } from "monaco-editor";
import type { PromptReference } from "@/lib/prompts/expand-prompt-references";

export type ScriptPlaceholder = {
  key: string;
  description: string;
  example: string;
  executor_types: string[];
};

/**
 * Keeps a language-level completion registration from serving another editor's
 * model. Monaco registers providers by language, so every editor must reject
 * completion requests that do not belong to its mounted model.
 */
export function scopeCompletionProviderToModel(
  provider: languages.CompletionItemProvider,
  model: editor.ITextModel,
): languages.CompletionItemProvider {
  return {
    ...provider,
    provideCompletionItems(candidateModel, position, context, token) {
      if (candidateModel !== model) return { suggestions: [] };
      return provider.provideCompletionItems(candidateModel, position, context, token);
    },
  };
}

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
      const textAfter = line.substring(position.column - 1);

      // Only trigger after `{{`
      if (!textBefore.endsWith("{{") && !textBefore.match(/\{\{[\w.]*$/)) {
        return { suggestions: [] };
      }

      // Find the range to replace (from {{ to cursor)
      const match = textBefore.match(/\{\{([\w.]*)$/);
      const startCol = match ? position.column - match[1].length : position.column;
      const existingPlaceholderSuffix = textAfter.match(/^([\w.]*)\}\}/);

      const range: IRange = {
        startLineNumber: position.lineNumber,
        startColumn: startCol,
        endLineNumber: position.lineNumber,
        endColumn: position.column + (existingPlaceholderSuffix?.[1].length ?? 0),
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
        insertText: existingPlaceholderSuffix ? p.key : p.key + "}}",
        range,
        sortText: String(i).padStart(3, "0"),
      }));

      return { suggestions };
    },
  };
}

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
    triggerCharacters: ["@", " "],
    provideCompletionItems(
      model: editor.ITextModel,
      position: { lineNumber: number; column: number },
    ): languages.ProviderResult<languages.CompletionList> {
      const line = model.getLineContent(position.lineNumber);
      const textBefore = line.substring(0, position.column - 1);
      const atIndex = textBefore.lastIndexOf("@");
      const atColumn = atIndex + 1;

      if (atIndex < 0 || !isMentionStartColumn(line, atColumn)) {
        return { suggestions: [] };
      }

      const namePrefix = textBefore.slice(atIndex + 1);
      const range: IRange = {
        startLineNumber: position.lineNumber,
        startColumn: position.column - namePrefix.length,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      };

      const suggestions: languages.CompletionItem[] = prompts.map((prompt, i) => ({
        label: `@${prompt.name}`,
        filterText: prompt.name,
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
