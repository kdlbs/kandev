import { isMarkdownFile } from "@/lib/utils/file-types";
import type { MarkdownFileMode } from "@/lib/types/workspace-files";

export type { MarkdownFileMode } from "@/lib/types/workspace-files";

export type StoredMarkdownFileMode = {
  markdownMode?: MarkdownFileMode;
  markdownPreview?: boolean;
};

/** Mode for a newly opened Markdown file. Non-Markdown files have no mode. */
export function defaultMarkdownFileMode(path: string): MarkdownFileMode | undefined {
  return isMarkdownFile(path) ? "preview" : undefined;
}

/**
 * Resolve a persisted tab. Missing legacy state means Source because the tab
 * was already open before the Markdown preview feature existed.
 */
export function resolveStoredMarkdownFileMode(stored: StoredMarkdownFileMode): MarkdownFileMode {
  if (stored.markdownMode && isMarkdownFileMode(stored.markdownMode)) {
    return stored.markdownMode;
  }
  return stored.markdownPreview === true ? "preview" : "source";
}

export function isMarkdownFileModeSupported(path: string, mode: MarkdownFileMode): boolean {
  if (!isMarkdownFile(path)) return false;
  return !path.toLowerCase().endsWith(".mdx") || mode !== "edit";
}

export function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function isMarkdownFileMode(value: string): value is MarkdownFileMode {
  return value === "preview" || value === "edit" || value === "source";
}
