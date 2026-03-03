import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, requestFileContentAtRef } from "@/lib/ws/workspace-files";

/** Must match @pierre/diffs SPLIT_WITH_NEWLINES — splits preserving trailing \n */
const SPLIT_WITH_NEWLINES = /(?<=\n)/;

type UseExpandableDiffOptions = {
  sessionId: string | undefined;
  filePath: string;
  baseRef: string | undefined;
  fileDiffMetadata: FileDiffMetadata | null;
  enableExpansion?: boolean;
};

type UseExpandableDiffResult = {
  metadata: FileDiffMetadata | null;
  isContentLoaded: boolean;
  isLoading: boolean;
  error: string | null;
  loadContent: () => Promise<void>;
  canExpand: boolean;
};

type WsClient = NonNullable<ReturnType<typeof getWebSocketClient>>;

/** Fetch old file content at a git ref. Returns empty string for new files. */
async function fetchOldContent(
  client: WsClient,
  sessionId: string,
  filePath: string,
  baseRef: string,
): Promise<string> {
  const res = await requestFileContentAtRef(client, sessionId, filePath, baseRef);
  if (res.is_binary) throw new Error("Cannot expand binary files");
  if (!res.error) return res.content;
  if (res.error.includes("file not found at ref")) return "";
  throw new Error(res.error);
}

/** Fetch new file content. Returns empty string for deleted files. */
async function fetchNewContent(client: WsClient, sessionId: string, filePath: string) {
  const res = await requestFileContent(client, sessionId, filePath);
  if (res.is_binary) throw new Error("Cannot expand binary files");
  if (res.error && /not found|no such file/i.test(res.error)) return "";
  if (res.error) throw new Error(res.error);
  return res.content;
}

/** Fetch both old and new content, split into lines for @pierre/diffs expansion. */
async function fetchExpansionLines(
  sessionId: string,
  filePath: string,
  baseRef: string | undefined,
) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  const newContent = await fetchNewContent(client, sessionId, filePath);
  const oldContent = baseRef ? await fetchOldContent(client, sessionId, filePath, baseRef) : "";
  return {
    oldLines: oldContent.split(SPLIT_WITH_NEWLINES),
    newLines: newContent.split(SPLIT_WITH_NEWLINES),
  };
}

/**
 * Hook for managing expandable diffs with lazy-loaded file content.
 *
 * The @pierre/diffs library requires `oldLines` and `newLines` in FileDiffMetadata
 * for expansion to work. This hook fetches old/new content and merges it into the
 * metadata.
 */
export function useExpandableDiff({
  sessionId,
  filePath,
  baseRef,
  fileDiffMetadata,
  enableExpansion = false,
}: UseExpandableDiffOptions): UseExpandableDiffResult {
  const requestVersionRef = useRef(0);
  const [loadedContent, setLoadedContent] = useState<{
    oldLines: string[];
    newLines: string[];
  } | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset cached content when inputs change so stale data is never rendered.
  useEffect(() => {
    requestVersionRef.current += 1;
    setLoadedContent(null);
    setError(null);
  }, [sessionId, filePath, baseRef]);

  const loadContent = useCallback(async () => {
    if (!sessionId || !enableExpansion || loadedContent || isLoading) return;

    const version = ++requestVersionRef.current;
    setIsLoading(true);
    setError(null);

    try {
      const lines = await fetchExpansionLines(sessionId, filePath, baseRef);
      if (version === requestVersionRef.current) setLoadedContent(lines);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load file content";
      console.error("[useExpandableDiff]", msg);
      if (version === requestVersionRef.current) setError(msg);
    } finally {
      setIsLoading(false);
    }
  }, [sessionId, filePath, baseRef, enableExpansion, loadedContent, isLoading]);

  const metadata = useMemo<FileDiffMetadata | null>(() => {
    if (!fileDiffMetadata) return null;
    if (!loadedContent) return fileDiffMetadata;
    return {
      ...fileDiffMetadata,
      oldLines: loadedContent.oldLines,
      newLines: loadedContent.newLines,
    };
  }, [fileDiffMetadata, loadedContent]);

  const isContentLoaded = loadedContent !== null;

  return {
    metadata,
    isContentLoaded,
    isLoading,
    error,
    loadContent,
    canExpand: enableExpansion && isContentLoaded && !error,
  };
}
