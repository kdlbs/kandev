'use client';

import { useRef, useEffect, useCallback } from 'react';
import { Input } from '@kandev/ui/input';
import { FileIcon } from '@/components/ui/file-icon';

type InlineFileInputProps = {
  depth: number;
  onSubmit: (name: string) => void;
  onCancel: () => void;
};

export function InlineFileInput({ depth, onSubmit, onCancel }: InlineFileInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // Focus on next tick to ensure the element is rendered
    requestAnimationFrame(() => inputRef.current?.focus());
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        const value = inputRef.current?.value.trim();
        if (value) onSubmit(value);
        else onCancel();
      } else if (e.key === 'Escape') {
        onCancel();
      }
    },
    [onSubmit, onCancel]
  );

  const handleBlur = useCallback(() => {
    const value = inputRef.current?.value.trim();
    if (value) onSubmit(value);
    else onCancel();
  }, [onSubmit, onCancel]);

  return (
    <div
      className="flex items-center gap-1 px-2 py-0.5"
      style={{ paddingLeft: `${depth * 12 + 8 + 20}px` }}
    >
      <FileIcon fileName="file" filePath="" className="flex-shrink-0" style={{ width: '14px', height: '14px', opacity: 0.7 }} />
      <Input
        ref={inputRef}
        type="text"
        className="h-5 text-xs px-1 py-0 border-muted-foreground/30"
        placeholder="filename..."
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
      />
    </div>
  );
}
