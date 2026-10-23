import {
  createDefaultMonacoSyntaxHighlighter,
  type IMonarchApi,
  type MonacoSyntaxHighlighter,
} from "@vscode/markdown-editor";
import { language as cssLanguage } from "monaco-editor/esm/vs/basic-languages/css/css.js";
import { language as htmlLanguage } from "monaco-editor/esm/vs/basic-languages/html/html.js";
import { language as javascriptLanguage } from "monaco-editor/esm/vs/basic-languages/javascript/javascript.js";
import { language as pythonLanguage } from "monaco-editor/esm/vs/basic-languages/python/python.js";
import { language as rustLanguage } from "monaco-editor/esm/vs/basic-languages/rust/rust.js";
import { language as shellLanguage } from "monaco-editor/esm/vs/basic-languages/shell/shell.js";
import { language as typescriptLanguage } from "monaco-editor/esm/vs/basic-languages/typescript/typescript.js";
import { language as yamlLanguage } from "monaco-editor/esm/vs/basic-languages/yaml/yaml.js";
import { compile } from "monaco-editor/esm/vs/editor/standalone/common/monarch/monarchCompile.js";
import { MonarchTokenizer } from "monaco-editor/esm/vs/editor/standalone/common/monarch/monarchLexer.js";

export function createMarkdownSyntaxHighlighter(): MonacoSyntaxHighlighter {
  return createDefaultMonacoSyntaxHighlighter(
    { compile, MonarchTokenizer } as unknown as IMonarchApi,
    {
      typescript: typescriptLanguage,
      javascript: javascriptLanguage,
      css: cssLanguage,
      html: htmlLanguage,
      python: pythonLanguage,
      rust: rustLanguage,
      shell: shellLanguage,
      yaml: yamlLanguage,
    },
  );
}
