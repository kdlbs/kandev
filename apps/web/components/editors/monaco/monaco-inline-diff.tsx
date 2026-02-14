'use client';

import { useMemo } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { useTheme } from 'next-themes';
import { cn } from '@/lib/utils';
import type { FileDiffData } from '@/lib/diff/types';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY } from '@/lib/theme/editor-theme';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

type MonacoInlineDiffProps = {
  data: FileDiffData;
  className?: string;
};

export function MonacoInlineDiff({ data, className }: MonacoInlineDiffProps) {
  const { resolvedTheme } = useTheme();
  const language = getMonacoLanguage(data.filePath);

  const { original, modified } = useMemo(() => {
    if (data.oldContent || data.newContent) {
      return {
        original: data.oldContent ?? '',
        modified: data.newContent ?? '',
      };
    }

    // Reconstruct from unified diff patch
    if (data.diff) {
      const originalLines: string[] = [];
      const modifiedLines: string[] = [];
      for (const line of data.diff.split('\n')) {
        if (line.startsWith('diff ') || line.startsWith('index ') ||
            line.startsWith('--- ') || line.startsWith('+++ ') ||
            line.startsWith('@@') || line.startsWith('\\')) continue;
        if (line.startsWith('-')) {
          originalLines.push(line.slice(1));
        } else if (line.startsWith('+')) {
          modifiedLines.push(line.slice(1));
        } else {
          const content = line.startsWith(' ') ? line.slice(1) : line;
          originalLines.push(content);
          modifiedLines.push(content);
        }
      }
      return { original: originalLines.join('\n'), modified: modifiedLines.join('\n') };
    }

    return { original: '', modified: '' };
  }, [data.oldContent, data.newContent, data.diff]);

  const lineCount = Math.max(
    original.split('\n').length,
    modified.split('\n').length
  );
  const height = Math.min(Math.max(lineCount * 16 + 8, 48), 300);

  return (
    <div className={cn('mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs overflow-hidden', className)}>
      <DiffEditor
        height={height}
        language={language}
        original={original}
        modified={modified}
        theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
        options={{
          readOnly: true,
          fontSize: 11,
          fontFamily: EDITOR_FONT_FAMILY,
          lineHeight: 16,
          minimap: { enabled: false },
          wordWrap: 'on',
          renderSideBySide: false,
          scrollBeyondLastLine: false,
          lineNumbers: 'off',
          glyphMargin: false,
          folding: false,
          renderOverviewRuler: false,
          scrollbar: { vertical: 'hidden', horizontal: 'hidden', alwaysConsumeMouseWheel: false },
          automaticLayout: true,
          domReadOnly: true,
          contextmenu: false,
          padding: { top: 2, bottom: 2 },
        }}
        loading={null}
      />
    </div>
  );
}
