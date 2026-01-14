'use client';

import CodeMirror from '@uiw/react-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { go } from '@codemirror/lang-go';
import { rust } from '@codemirror/lang-rust';
import { java } from '@codemirror/lang-java';
import { cpp } from '@codemirror/lang-cpp';
import { css } from '@codemirror/lang-css';
import { html } from '@codemirror/lang-html';
import { json } from '@codemirror/lang-json';
import { markdown } from '@codemirror/lang-markdown';
import { yaml } from '@codemirror/lang-yaml';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { EditorView } from '@codemirror/view';
import type { Extension } from '@codemirror/state';

type FileViewerContentProps = {
  path: string;
  content: string;
};

const getLanguageExtension = (path: string): Extension | undefined => {
  const ext = path.split('.').pop()?.toLowerCase();
  switch (ext) {
    case 'js':
    case 'mjs':
    case 'cjs':
      return javascript();
    case 'jsx':
      return javascript({ jsx: true });
    case 'ts':
    case 'mts':
    case 'cts':
      return javascript({ typescript: true });
    case 'tsx':
      return javascript({ jsx: true, typescript: true });
    case 'py':
      return python();
    case 'go':
      return go();
    case 'rs':
      return rust();
    case 'java':
      return java();
    case 'cpp':
    case 'cc':
    case 'cxx':
    case 'c':
    case 'h':
    case 'hpp':
      return cpp();
    case 'css':
    case 'scss':
    case 'less':
      return css();
    case 'html':
    case 'htm':
      return html();
    case 'json':
      return json();
    case 'md':
    case 'mdx':
      return markdown();
    case 'yaml':
    case 'yml':
      return yaml();
    default:
      return undefined;
  }
};

export function FileViewerContent({ path, content }: FileViewerContentProps) {
  const langExt = getLanguageExtension(path);
  const extensions: Extension[] = [
    EditorView.lineWrapping,
    EditorView.editable.of(false),
  ];
  if (langExt) {
    extensions.push(langExt);
  }

  return (
    <CodeMirror
      value={content}
      height="100%"
      theme={vscodeDark}
      extensions={extensions}
      readOnly
      basicSetup={{
        lineNumbers: true,
        foldGutter: true,
        highlightActiveLine: false,
        highlightSelectionMatches: true,
      }}
      className="h-full overflow-auto text-sm"
    />
  );
}

