/**
 * Unified file attachment type and processing utilities.
 * Handles both images (with preview) and arbitrary files (code, docs, etc.).
 */

import { generateUUID } from "@/lib/utils";
import { formatBytes } from "@/lib/utils/format-bytes";

export type FileAttachment = {
  id: string;
  /** Legacy inline data. New uploads use `file` + `attachmentId` instead. */
  data?: string;
  file?: File;
  attachmentId?: string;
  uploadStatus?: "pending" | "uploading" | "ready" | "failed";
  uploadError?: string;
  mimeType: string; // MIME type
  fileName: string; // Original file name
  size: number; // File size in bytes
  preview?: string; // Data URL for image preview (only for images)
  isImage: boolean; // Whether this is a previewable image
  deliveryMode: "prompt" | "path"; // Native prompt block or writable executor file path
};

export const PREVIEWABLE_IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

export const MAX_FILE_SIZE = 100 * 1024 * 1024; // 100 MiB per file
export const MAX_TOTAL_SIZE = 100 * 1024 * 1024; // 100 MiB total
export const MAX_FILES = 10;
const MAX_INLINE_PREVIEW_BYTES = 5 * 1024 * 1024;

function isPreviewableImage(mimeType: string): boolean {
  return PREVIEWABLE_IMAGE_TYPES.includes(mimeType);
}

/**
 * Process a file into a FileAttachment.
 * Small images get a preview data URL; larger files retain the File object so
 * the browser can stream them to the attachment endpoint without creating a
 * 100 MiB base64 copy in memory.
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

    const mimeType = file.type || "application/octet-stream";
    const isImage = isPreviewableImage(mimeType);
    if (file.size > MAX_INLINE_PREVIEW_BYTES) {
      resolve({
        id: generateUUID(),
        file,
        mimeType,
        fileName: file.name,
        size: file.size,
        isImage,
        deliveryMode: isImage ? "prompt" : "path",
        uploadStatus: "pending",
      });
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
      const resolvedMimeType =
        file.type || dataUrl.split(";")[0].split(":")[1] || "application/octet-stream";
      const resolvedIsImage = isPreviewableImage(resolvedMimeType);

      if (resolvedIsImage) {
        // For images, load to verify and generate preview
        const img = new Image();
        img.onload = () => {
          resolve({
            id: generateUUID(),
            data: base64,
            file,
            mimeType: resolvedMimeType,
            fileName: file.name,
            size: file.size,
            preview: dataUrl,
            isImage: true,
            deliveryMode: "prompt",
            uploadStatus: "pending",
          });
        };
        img.onerror = () => resolve(null);
        img.src = dataUrl;
      } else {
        resolve({
          id: generateUUID(),
          data: base64,
          file,
          mimeType: resolvedMimeType,
          fileName: file.name,
          size: file.size,
          isImage: false,
          deliveryMode: "path",
          uploadStatus: "pending",
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
