'use client';

import { ScrollArea } from '@kandev/ui/scroll-area';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';

type FileViewerContentProps = {
  path: string;
  content: string;
};

const getLanguageFromPath = (path: string): string => {
  const ext = path.split('.').pop()?.toLowerCase();
  const langMap: Record<string, string> = {
    js: 'javascript',
    jsx: 'jsx',
    ts: 'typescript',
    tsx: 'tsx',
    py: 'python',
    go: 'go',
    rs: 'rust',
    java: 'java',
    cpp: 'cpp',
    c: 'c',
    css: 'css',
    html: 'html',
    json: 'json',
    yaml: 'yaml',
    yml: 'yaml',
    md: 'markdown',
    sh: 'bash',
  };
  return langMap[ext || ''] || 'text';
};

export function FileViewerContent({ path, content }: FileViewerContentProps) {
  return (
    <ScrollArea className="h-full rounded-lg bg-background">
      <SyntaxHighlighter
        language={getLanguageFromPath(path)}
        style={vscDarkPlus}
        showLineNumbers
        customStyle={{ margin: 0, borderRadius: '0.5rem' }}
      >
        {content}
      </SyntaxHighlighter>
    </ScrollArea>
  );
}

