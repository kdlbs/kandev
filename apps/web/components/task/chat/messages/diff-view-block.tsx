'use client';

import { cn } from '@/lib/utils';
import { useEditorProvider } from '@/hooks/use-editor-resolver';
import { MonacoInlineDiff } from '@/components/editors/monaco/monaco-inline-diff';
import { DiffViewInline } from '@/components/diff';
import type { FileDiffData } from '@/lib/diff/types';

type DiffViewBlockProps = {
  data: FileDiffData;
  className?: string;
};

export function DiffViewBlock({ data, className }: DiffViewBlockProps) {
  const provider = useEditorProvider('chat-diff');

  if (provider === 'monaco') {
    return <MonacoInlineDiff data={data} className={className} />;
  }

  return (
    <div className={cn("mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs", className)}>
      <DiffViewInline data={data} />
    </div>
  );
}
