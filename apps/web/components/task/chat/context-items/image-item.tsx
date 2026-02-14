'use client';

import { memo, useState, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from '@kandev/ui/dialog';
import type { ImageContextItem } from '@/lib/types/context';
import { ContextChip } from './context-chip';

export const ImageItem = memo(function ImageItem({ item }: { item: ImageContextItem }) {
  const [dialogOpen, setDialogOpen] = useState(false);

  const handleClick = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const preview = (
    <div className="space-y-1.5">
      {/* eslint-disable-next-line @next/next/no-img-element -- base64 preview */}
      <img
        src={item.attachment.preview}
        alt="Preview"
        className="max-w-full max-h-48 rounded object-contain"
      />
      <div className="text-xs text-muted-foreground">
        {item.attachment.width} &times; {item.attachment.height}
      </div>
    </div>
  );

  return (
    <>
      <ContextChip
        kind="image"
        label={item.label}
        thumbnail={item.attachment.preview}
        preview={preview}
        onClick={handleClick}
        onRemove={item.onRemove}
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-3xl p-2">
          <DialogTitle className="sr-only">Image preview</DialogTitle>
          {/* eslint-disable-next-line @next/next/no-img-element -- base64 preview */}
          <img
            src={item.attachment.preview}
            alt="Full size preview"
            className="max-w-full max-h-[80vh] rounded object-contain mx-auto"
          />
        </DialogContent>
      </Dialog>
    </>
  );
});
