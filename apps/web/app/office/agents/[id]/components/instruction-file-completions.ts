import type { editor, IRange, languages } from "monaco-editor";

/**
 * Builds a Monaco completion provider that suggests sibling instruction
 * filenames when the user types a relative path like `./` or `../`. The
 * provider triggers on `/` because that's the character immediately
 * preceding a path segment in markdown links and bare references.
 */
export function createInstructionFileCompletionProvider(
  monaco: typeof import("monaco-editor"),
  filenames: string[],
): languages.CompletionItemProvider {
  return {
    triggerCharacters: ["/", "."],
    provideCompletionItems(
      model: editor.ITextModel,
      position: { lineNumber: number; column: number },
    ): languages.ProviderResult<languages.CompletionList> {
      const line = model.getLineContent(position.lineNumber);
      const textBefore = line.substring(0, position.column - 1);

      // Match the in-progress relative ref ending at the cursor:
      //   ./<chars>   or   ../<chars>
      // We also accept a bare leading `.` (user just typed `./` partially).
      const match = textBefore.match(/(\.{1,2}\/)([\w.-]*)$/);
      if (!match) return { suggestions: [] };

      const prefix = match[1]; // "./" or "../"
      const partial = match[2];
      const replaceFrom = position.column - partial.length;

      const range: IRange = {
        startLineNumber: position.lineNumber,
        startColumn: replaceFrom,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      };

      const suggestions: languages.CompletionItem[] = filenames
        .filter((name) => name.toLowerCase().startsWith(partial.toLowerCase()))
        .map((name) => ({
          label: `${prefix}${name}`,
          kind: monaco.languages.CompletionItemKind.File,
          detail: "instruction file",
          insertText: name,
          range,
        }));

      return { suggestions };
    },
  };
}
