import { useState, useCallback, useMemo } from "react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, requestFileContentAtRef } from "@/lib/ws/workspace-files";

type UseExpandableDiffOptions = {
  /** Session ID for WebSocket requests */
  sessionId: string | undefined;
  /** The file path */
  filePath: string;
  /** Base git ref for fetching old content (e.g., "origin/main", "HEAD~1") */
  baseRef: string | undefined;
  /** The parsed diff metadata */
  fileDiffMetadata: FileDiffMetadata | null;
  /** Whether expansion is enabled */
  enableExpansion?: boolean;
};

type UseExpandableDiffResult = {
  /** The file diff metadata with oldLines/newLines if loaded */
  metadata: FileDiffMetadata | null;
  /** Whether the full file content has been loaded for expansion */
  isContentLoaded: boolean;
  /** Whether content is currently being loaded */
  isLoading: boolean;
  /** Error message if content loading failed */
  error: string | null;
  /** Load full file content for expansion. Called automatically on first expand. */
  loadContent: () => Promise<void>;
  /** Whether expansion is available (content loaded or can be loaded) */
  canExpand: boolean;
};

/**
 * Hook for managing expandable diffs with lazy-loaded file content.
 *
 * The @pierre/diffs library requires `oldLines` and `newLines` in FileDiffMetadata
 * for expansion to work. This hook:
 * - Tracks whether full content has been loaded
 * - Provides a loadContent function to fetch old/new file content
 * - Merges loaded content into the metadata for expansion
 */
export function useExpandableDiff({
  sessionId,
  filePath,
  baseRef,
  fileDiffMetadata,
  enableExpansion = false,
}: UseExpandableDiffOptions): UseExpandableDiffResult {
  const [loadedContent, setLoadedContent] = useState<{
    oldLines: string[];
    newLines: string[];
  } | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadContent = useCallback(async () => {
    if (!sessionId || !enableExpansion || loadedContent || isLoading) {
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const client = getWebSocketClient();
      if (!client) {
        throw new Error("WebSocket client not available");
      }

      // Fetch current file content (new version)
      const newContentResponse = await requestFileContent(client, sessionId, filePath);
      if (newContentResponse.error || newContentResponse.is_binary) {
        throw new Error(newContentResponse.error || "Cannot expand binary files");
      }

      let oldContent = "";
      // Fetch old file content at base ref (if provided)
      if (baseRef) {
        try {
          const oldContentResponse = await requestFileContentAtRef(
            client,
            sessionId,
            filePath,
            baseRef,
          );
          if (!oldContentResponse.error && !oldContentResponse.is_binary) {
            oldContent = oldContentResponse.content;
          }
          // If old content fails (file is new), use empty string
        } catch {
          // File doesn't exist at base ref - this is fine for new files
          oldContent = "";
        }
      }

      setLoadedContent({
        oldLines: oldContent.split("\n"),
        newLines: newContentResponse.content.split("\n"),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load file content");
    } finally {
      setIsLoading(false);
    }
  }, [sessionId, filePath, baseRef, enableExpansion, loadedContent, isLoading]);

  // Merge loaded content into metadata
  const metadata = useMemo<FileDiffMetadata | null>(() => {
    if (!fileDiffMetadata) return null;

    if (loadedContent) {
      return {
        ...fileDiffMetadata,
        oldLines: loadedContent.oldLines,
        newLines: loadedContent.newLines,
      };
    }

    return fileDiffMetadata;
  }, [fileDiffMetadata, loadedContent]);

  const isContentLoaded = loadedContent !== null;
  const canExpand = enableExpansion && (isContentLoaded || (!!sessionId && !error));

  return {
    metadata,
    isContentLoaded,
    isLoading,
    error,
    loadContent,
    canExpand,
  };
}
