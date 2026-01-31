'use client';

import { useEffect, useRef, useState } from 'react';
import { useTheme } from 'next-themes';
import { Editor, rootCtx, defaultValueCtx } from '@milkdown/core';
import { nord } from '@milkdown/theme-nord';
import { commonmark } from '@milkdown/preset-commonmark';
import { gfm } from '@milkdown/preset-gfm';
import { listener, listenerCtx } from '@milkdown/plugin-listener';
import { Milkdown, MilkdownProvider, useEditor } from '@milkdown/react';

type MilkdownEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
};

function MilkdownEditorInner({ value, onChange }: MilkdownEditorProps) {
  const { resolvedTheme } = useTheme();
  // Capture initial value at mount - editor manages its own state after that
  const initialValueRef = useRef(value);
  const onChangeRef = useRef(onChange);

  // Keep onChange ref updated
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  const { loading } = useEditor((root) =>
    Editor.make()
      .config((ctx) => {
        ctx.set(rootCtx, root);
        ctx.set(defaultValueCtx, initialValueRef.current);
        ctx.get(listenerCtx).markdownUpdated((_, markdown) => {
          onChangeRef.current(markdown);
        });
      })
      .config(nord)
      .use(commonmark)
      .use(gfm)
      .use(listener)
  );

  return (
    <div className={`milkdown-wrapper h-full relative ${resolvedTheme === 'dark' ? 'dark' : ''}`}>
      <Milkdown />
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center text-muted-foreground text-sm bg-background/80">
          Loading editor...
        </div>
      )}
    </div>
  );
}

export function MarkdownEditor(props: MilkdownEditorProps) {
  // Generate a unique key on each mount to force fresh Milkdown instance
  const [instanceId] = useState(() => Math.random().toString(36).slice(2));

  return (
    <MilkdownProvider key={instanceId}>
      <MilkdownEditorInner {...props} />
    </MilkdownProvider>
  );
}

