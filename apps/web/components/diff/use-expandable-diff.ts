import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { processFile, type FileDiffMetadata } from "@pierre/diffs";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, requestFileContentAtRef } from "@/lib/ws/workspace-files";

type UseExpandableDiffOptions = {
  sessionId: string | undefined;
  filePath: string;
  baseRef: string | undefined;
  fileDiffMetadata: FileDiffMetadata | null;
  /** Original patch string used to build fileDiffMetadata. Required for
   *  re-parsing via processFile with the loaded full content. */
  diff: string | undefined;
  enableExpansion?: boolean;
  /** Multi-repo subpath for the file (e.g. "kandev"); empty for single-repo. */
  repo?: string;
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

/** Check if an error indicates file not found (various formats from backend). */
function isFileNotFoundError(error: string): boolean {
  return /file not found|not found|no such file|does not exist/i.test(error);
}

/** Fetch old file content at a git ref. Returns empty string for new files. */
async function fetchOldContent(
  client: WsClient,
  sessionId: string,
  filePath: string,
  baseRef: string,
  repo?: string,
): Promise<string> {
  try {
    const res = await requestFileContentAtRef(client, sessionId, filePath, baseRef, repo);
    if (res.is_binary) throw new Error("Cannot expand binary files");
    if (!res.error) return res.content;
    // File not found at ref is expected for new files - return empty string
    if (isFileNotFoundError(res.error)) return "";
    throw new Error(res.error);
  } catch (err) {
    // WebSocket client throws errors for backend error responses
    const msg = err instanceof Error ? err.message : String(err);
    if (isFileNotFoundError(msg)) return "";
    throw err;
  }
}

/** Fetch new file content from the working tree. Returns empty string for deleted files. */
async function fetchNewContent(
  client: WsClient,
  sessionId: string,
  filePath: string,
  repo?: string,
): Promise<string> {
  try {
    // Fetch from working tree (current file on disk), not HEAD.
    // The diff shows working tree changes, so additionLines must match.
    const res = await requestFileContent(client, sessionId, filePath, repo);
    if (res.is_binary) throw new Error("Cannot expand binary files");
    if (!res.error) return res.content;
    // File not found is expected for deleted files - return empty string
    if (isFileNotFoundError(res.error)) return "";
    throw new Error(res.error);
  } catch (err) {
    // WebSocket client throws errors for backend error responses
    const msg = err instanceof Error ? err.message : String(err);
    if (isFileNotFoundError(msg)) return "";
    throw err;
  }
}

/** Fetch both old and new content as raw strings for @pierre/diffs expansion. */
async function fetchExpansionContent(
  sessionId: string,
  filePath: string,
  baseRef: string | undefined,
  repo: string | undefined,
) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  const newContent = await fetchNewContent(client, sessionId, filePath, repo);
  const oldContent = baseRef
    ? await fetchOldContent(client, sessionId, filePath, baseRef, repo)
    : "";
  return { oldContent, newContent };
}

/**
 * Hook for managing expandable diffs with lazy-loaded file content.
 *
 * @pierre/diffs needs the patch *and* the full file contents (with
 * isPartial=false and hunk indices addressed against the full arrays) to
 * render expand controls. We get there by re-parsing via `processFile`
 * once the content arrives — it's the only API that produces a metadata
 * shape internally consistent enough for the library's expansion path.
 */
export function useExpandableDiff({
  sessionId,
  filePath,
  baseRef,
  fileDiffMetadata,
  diff,
  enableExpansion = false,
  repo,
}: UseExpandableDiffOptions): UseExpandableDiffResult {
  const requestVersionRef = useRef(0);
  const [loadedContent, setLoadedContent] = useState<{
    oldContent: string;
    newContent: string;
  } | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset cached content when inputs change so stale data is never rendered.
  // Including fileDiffMetadata ensures expansion content is invalidated when
  // the diff changes (e.g., file modified while Diff panel is open).
  useEffect(() => {
    requestVersionRef.current += 1;
    setLoadedContent(null);
    setError(null);
  }, [sessionId, filePath, baseRef, repo, fileDiffMetadata]);

  const loadContent = useCallback(async () => {
    if (!sessionId || !enableExpansion || loadedContent || isLoading) return;

    const version = ++requestVersionRef.current;
    setIsLoading(true);
    setError(null);

    try {
      const content = await fetchExpansionContent(sessionId, filePath, baseRef, repo);
      if (version === requestVersionRef.current) setLoadedContent(content);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load file content";
      console.error("[useExpandableDiff]", msg);
      if (version === requestVersionRef.current) setError(msg);
    } finally {
      setIsLoading(false);
    }
  }, [sessionId, filePath, baseRef, repo, enableExpansion, loadedContent, isLoading]);

  const metadata = useMemo<FileDiffMetadata | null>(() => {
    if (!fileDiffMetadata) return null;
    if (!loadedContent || !diff) return fileDiffMetadata;
    const reparsed = processFile(diff, {
      oldFile: { name: filePath, contents: loadedContent.oldContent },
      newFile: { name: filePath, contents: loadedContent.newContent },
    });
    if (!reparsed) return fileDiffMetadata;
    // Preserve the lang override that useDiffMetadata sets (e.g. lang:'text'
    // for Go files that hit the Shiki backtracking guard). processFile would
    // otherwise infer "go" from the filename and silently re-enable Shiki.
    return fileDiffMetadata.lang ? { ...reparsed, lang: fileDiffMetadata.lang } : reparsed;
  }, [fileDiffMetadata, loadedContent, diff, filePath]);

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
