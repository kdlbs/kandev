/**
 * Unified file attachment type and processing utilities.
 * Handles both images (with preview) and arbitrary files (code, docs, etc.).
 */

import { generateUUID } from "@/lib/utils";

export type FileAttachment = {
  id: string;
  data: string; // Base64-encoded content (without data: prefix)
  mimeType: string; // MIME type
  fileName: string; // Original file name
  size: number; // File size in bytes
  preview?: string; // Data URL for image preview (only for images)
  isImage: boolean; // Whether this is a previewable image
};

export const PREVIEWABLE_IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

export const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB per file
export const MAX_TOTAL_SIZE = 20 * 1024 * 1024; // 20MB total
export const MAX_FILES = 10;

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function isPreviewableImage(mimeType: string): boolean {
  return PREVIEWABLE_IMAGE_TYPES.includes(mimeType);
}

/**
 * Process a file into a FileAttachment.
 * Images get a preview data URL; other files just get base64 data.
 * Returns null if the file is invalid (too large, etc.)
 */
export function processFile(file: File): Promise<FileAttachment | null> {
  return new Promise((resolve) => {
    if (file.size > MAX_FILE_SIZE) {
      console.warn(
        `File too large: ${formatBytes(file.size)} (max: ${formatBytes(MAX_FILE_SIZE)})`,
      );
      resolve(null);
      return;
    }

    // Skip directories and empty files
    if (file.size === 0 && file.type === "") {
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

      const base64 = dataUrl.split(",")[1];
      const mimeType =
        file.type || dataUrl.split(";")[0].split(":")[1] || "application/octet-stream";
      const isImage = isPreviewableImage(mimeType);

      if (isImage) {
        // For images, load to verify and generate preview
        const img = new Image();
        img.onload = () => {
          resolve({
            id: generateUUID(),
            data: base64,
            mimeType,
            fileName: file.name,
            size: file.size,
            preview: dataUrl,
            isImage: true,
          });
        };
        img.onerror = () => resolve(null);
        img.src = dataUrl;
      } else {
        resolve({
          id: generateUUID(),
          data: base64,
          mimeType,
          fileName: file.name,
          size: file.size,
          isImage: false,
        });
      }
    };
    reader.onerror = () => {
      console.error("Failed to read file:", file.name);
      resolve(null);
    };
    reader.readAsDataURL(file);
  });
}
