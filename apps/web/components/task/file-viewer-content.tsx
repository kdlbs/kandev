'use client';

import CodeMirror from '@uiw/react-codemirror';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { EditorView } from '@codemirror/view';
import type { Extension } from '@codemirror/state';
import { getCodeMirrorExtensionFromPath } from '@/lib/languages';

type FileViewerContentProps = {
  path: string;
  content: string;
};

export function FileViewerContent({ path, content }: FileViewerContentProps) {
  const langExt = getCodeMirrorExtensionFromPath(path);
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
      className="h-full overflow-auto text-xs"
    />
  );
}

