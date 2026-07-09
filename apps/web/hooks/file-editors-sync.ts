"use client";

import { useEffect } from "react";
import type { FileEditorState } from "@/lib/state/dockview-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { calculateHash } from "@/lib/utils/file-diff";
import { updatePanelAfterSave } from "./use-file-save-delete";
import type { FileInfo } from "@/lib/state/store";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

/**
 * Builds a stable signature string from a file's git status entry. The hook
 * compares signatures across renders to decide whether the editor's content
 * needs re-syncing from the workspace.
 */
export function buildGitFileSignature(file: FileInfo | undefined): string {
  if (!file) return "__clean__";
  return [
    file.status ?? "",
    file.staged ? "1" : "0",
    String(file.additions ?? 0),
    String(file.deletions ?? 0),
    file.old_path ?? "",
    file.diff ?? "",
  ].join("|");
}

export type SyncOpenFileArgs = {
  client: ReturnType<typeof getWebSocketClient>;
  sessionId: string;
  fileKey: string;
  path: string;
  repo?: string;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
};

/**
 * Re-fetches file content from the workspace and reconciles it with the open
 * editor buffer. Behavior depends on dirty state:
 *  - Clean buffer: replaces `content` + `originalContent` so the editor shows
 *    the latest disk state without losing user focus/cursor.
 *  - Dirty buffer with matching remote content: clears dirty flag (the user's
 *    edits happen to equal what landed on disk).
 *  - Dirty buffer with different remote content: surfaces a "Reload" affordance
 *    via `hasRemoteUpdate` so the user explicitly chooses to clobber edits.
 */
export async function syncOpenFileFromWorkspace({
  client,
  sessionId,
  fileKey,
  path,
  repo,
  updateFileState,
}: SyncOpenFileArgs): Promise<void> {
  if (!client) return;
  try {
    const response = await requestFileContent(client, sessionId, path, repo);
    const latest = useDockviewStore.getState().openFiles.get(fileKey);
    if (!latest) return;
    // The tab may have been swapped to a different repo's file at the same path
    // key while the fetch was in flight; the response is then for the old repo.
    // Drop it rather than writing stale content into the new buffer.
    if (latest.repo !== repo) return;
    const remoteHash = await calculateHash(response.content);

    if (latest.isDirty) {
      if (response.content === latest.content) {
        updateFileState(fileKey, {
          originalContent: response.content,
          originalHash: remoteHash,
          isDirty: false,
          hasRemoteUpdate: false,
          remoteContent: undefined,
          remoteOriginalHash: undefined,
        });
        updatePanelAfterSave(path, latest.name, latest.repo);
        return;
      }
      if (latest.hasRemoteUpdate && latest.remoteContent === response.content) return;
      updateFileState(fileKey, {
        hasRemoteUpdate: true,
        remoteContent: response.content,
        remoteOriginalHash: remoteHash,
      });
      return;
    }

    if (
      latest.content === response.content &&
      latest.originalHash === remoteHash &&
      !latest.hasRemoteUpdate
    ) {
      return;
    }

    updateFileState(fileKey, {
      content: response.content,
      originalContent: response.content,
      originalHash: remoteHash,
      isDirty: false,
      isBinary: response.is_binary,
      hasRemoteUpdate: false,
      remoteContent: undefined,
      remoteOriginalHash: undefined,
    });
  } catch {
    // Ignore sync failures; user can continue editing.
  }
}

export type OpenFileWorkspaceSyncParams = {
  gitStatus: GitStatusEntry | undefined;
  openFiles: Map<string, FileEditorState>;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  gitFileSignaturesRef: React.MutableRefObject<Map<string, string>>;
};

/**
 * Watches gitStatus + openFiles and refetches open editor buffers from disk
 * whenever a file's git signature changes (the agent edited it, the user ran
 * git pull, etc.). Skips the very first observation per file so that opening
 * a file doesn't trigger an immediate redundant refetch.
 */
export function useOpenFileWorkspaceSync({
  gitStatus,
  openFiles,
  updateFileState,
  activeSessionIdRef,
  gitFileSignaturesRef,
}: OpenFileWorkspaceSyncParams) {
  useEffect(() => {
    const sigMap = gitFileSignaturesRef.current;
    for (const path of Array.from(sigMap.keys())) {
      if (!openFiles.has(path)) sigMap.delete(path);
    }
  }, [openFiles, gitFileSignaturesRef]);

  useEffect(() => {
    const client = getWebSocketClient();
    const sessionId = activeSessionIdRef.current;
    if (!client || !sessionId) return;

    const gitFiles = gitStatus?.files ?? {};
    const sigMap = gitFileSignaturesRef.current;
    for (const [fileKey, file] of openFiles.entries()) {
      if ((gitStatus?.repository_name ?? "") !== (file.repo ?? "")) continue;
      // For symlinks, also check the resolved target path in git status
      const gitFileInfo = (gitFiles[file.path] ??
        (file.resolvedPath ? gitFiles[file.resolvedPath] : undefined)) as FileInfo | undefined;
      const nextSignature = buildGitFileSignature(gitFileInfo);
      const prevSignature = sigMap.get(fileKey);
      if (prevSignature === undefined) {
        sigMap.set(fileKey, nextSignature);
        continue;
      }
      if (prevSignature === nextSignature) continue;

      sigMap.set(fileKey, nextSignature);
      void syncOpenFileFromWorkspace({
        client,
        sessionId,
        fileKey,
        path: file.path,
        repo: file.repo,
        updateFileState,
      });
    }
  }, [gitStatus, openFiles, updateFileState, activeSessionIdRef, gitFileSignaturesRef]);
}
