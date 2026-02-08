'use client';

import { memo } from 'react';
import { IconX } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

export type ImageAttachment = {
  id: string;
  data: string;       // Base64-encoded image data (without data: prefix)
  mimeType: string;   // "image/png", "image/jpeg", "image/gif", "image/webp"
  preview: string;    // Data URL for preview display
  size: number;       // File size in bytes
};

// Supported image types
export const SUPPORTED_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

// Size limits
export const MAX_IMAGE_SIZE = 10 * 1024 * 1024; // 10MB per image
export const MAX_TOTAL_SIZE = 20 * 1024 * 1024; // 20MB total
export const MAX_IMAGES = 10;

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

type ImageAttachmentPreviewProps = {
  attachments: ImageAttachment[];
  onRemove: (id: string) => void;
  disabled?: boolean;
};

export const ImageAttachmentPreview = memo(function ImageAttachmentPreview({
  attachments,
  onRemove,
  disabled = false,
}: ImageAttachmentPreviewProps) {
  if (attachments.length === 0) return null;

  return (
    <div className="flex gap-1.5 px-3 pt-2 flex-wrap">
      {attachments.map((attachment) => (
        <div
          key={attachment.id}
          className={cn(
            'relative group rounded-md overflow-hidden border border-border bg-muted/30',
            disabled && 'opacity-50'
          )}
        >
          {/* eslint-disable-next-line @next/next/no-img-element -- base64/blob preview URLs are not compatible with next/image */}
          <img
            src={attachment.preview}
            alt="Attachment preview"
            className="h-10 w-10 object-cover"
          />

          {/* Remove button - inside the image bounds */}
          {!disabled && (
            <button
              type="button"
              onClick={() => onRemove(attachment.id)}
              className={cn(
                'absolute top-0.5 right-0.5 p-0.5 rounded-full',
                'bg-black/70 text-white',
                'opacity-0 group-hover:opacity-100 transition-opacity',
                'hover:bg-black/90 cursor-pointer',
                'focus:outline-none focus:ring-1 focus:ring-white/50'
              )}
              aria-label="Remove image"
            >
              <IconX className="h-2.5 w-2.5" />
            </button>
          )}
        </div>
      ))}
    </div>
  );
});

/**
 * Process a pasted or dropped file into an ImageAttachment
 * Returns null if the file is invalid (wrong type, too large, etc.)
 */
export function processImageFile(file: File): Promise<ImageAttachment | null> {
  return new Promise((resolve) => {
    // Validate type
    if (!SUPPORTED_IMAGE_TYPES.includes(file.type)) {
      console.warn(`Unsupported image type: ${file.type}`);
      resolve(null);
      return;
    }

    // Validate size
    if (file.size > MAX_IMAGE_SIZE) {
      console.warn(`Image too large: ${formatBytes(file.size)} (max: ${formatBytes(MAX_IMAGE_SIZE)})`);
      resolve(null);
      return;
    }

    const reader = new FileReader();
    reader.onload = (event) => {
      const dataUrl = event.target?.result as string;
      if (!dataUrl) {
        resolve(null);
        return;
      }

      // Extract base64 data (remove "data:image/png;base64," prefix)
      const base64 = dataUrl.split(',')[1];
      const mimeType = dataUrl.split(';')[0].split(':')[1];

      resolve({
        id: crypto.randomUUID(),
        data: base64,
        mimeType,
        preview: dataUrl,
        size: file.size,
      });
    };
    reader.onerror = () => {
      console.error('Failed to read image file');
      resolve(null);
    };
    reader.readAsDataURL(file);
  });
}

