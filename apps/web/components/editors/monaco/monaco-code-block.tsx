'use client';

import { useTheme } from 'next-themes';
import Editor from '@monaco-editor/react';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { getMonacoLanguageFromName } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY } from '@/lib/theme/editor-theme';
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

type MonacoCodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

export function MonacoCodeBlock({ children, className }: MonacoCodeBlockProps) {
  const { copied, copy } = useCopyToClipboard();
  const { resolvedTheme } = useTheme();

  const code = String(children).replace(/\n$/, '');
  const lang = className?.replace('language-', '').toLowerCase() ?? '';
  const language = getMonacoLanguageFromName(lang);
  const lineCount = code.split('\n').length;
  // Compact height: ~16px per line + 8px padding
  const height = Math.min(Math.max(lineCount * 16 + 8, 32), 500);

  return (
    <div className="relative group/code-block my-4 w-fit max-w-full min-w-[50%]">
      <button
        onClick={() => copy(code)}
        className={cn(
          'absolute top-2 right-2 z-10',
          'p-1.5 rounded-md',
          'bg-white/10 hover:bg-white/20',
          'transition-all duration-200',
          'opacity-0 group-hover/code-block:opacity-100',
          'cursor-pointer',
        )}
        title="Copy code"
      >
        {copied ? (
          <IconCheck className="h-3 w-3 text-green-400" />
        ) : (
          <IconCopy className="h-3 w-3 text-gray-400" />
        )}
      </button>

      <Editor
        height={height}
        language={language}
        value={code}
        theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
        options={{
          readOnly: true,
          fontSize: 12,
          fontFamily: EDITOR_FONT_FAMILY,
          lineHeight: 16,
          minimap: { enabled: false },
          wordWrap: 'on',
          scrollBeyondLastLine: false,
          lineNumbers: 'off',
          glyphMargin: false,
          folding: false,
          renderLineHighlight: 'none',
          scrollbar: { vertical: 'hidden', horizontal: 'hidden' },
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          overviewRulerBorder: false,
          automaticLayout: true,
          domReadOnly: true,
          contextmenu: false,
          padding: { top: 4, bottom: 4 },
        }}
        className="rounded-md overflow-hidden text-xs"
        loading={null}
      />
    </div>
  );
}
