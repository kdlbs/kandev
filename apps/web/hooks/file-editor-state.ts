"use client";

import type { MutableRefObject } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { calculateHash } from "@/lib/utils/file-diff";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import type { FileContentResponse } from "@/lib/types/backend";

type DockApi = ReturnType<typeof useDockviewStore.getState>["api"];

export type FileEditorRequestToken = {
  fileKey: string;
  generation: number;
};

/**
 * Build a FileEditorState from a file content response. `repo` is the
 * multi-repo subpath (repository_name) the file belongs to; it is recorded on
 * the state so subsequent save/sync/delete requests stay scoped to the right
 * repository instead of resolving against the bare task root.
 */
export async function buildFileEditorState(
  filePath: string,
  response: FileContentResponse,
  repo?: string,
): Promise<FileEditorState> {
  const fileName = filePath.split("/").pop() || filePath;
  const hash = await calculateHash(response.content);
  return {
    path: filePath,
    repo,
    name: fileName,
    content: response.content,
    originalContent: response.content,
    originalHash: hash,
    isDirty: false,
    isBinary: response.is_binary,
    resolvedPath: response.resolved_path,
  };
}

type FetchFileEditorStateParams = {
  client: NonNullable<ReturnType<typeof getWebSocketClient>>;
  sessionId: string;
  filePath: string;
  repo: string | undefined;
  activeSessionIdRef: MutableRefObject<string | null>;
  activeRequestRef?: MutableRefObject<FileEditorRequestToken | null>;
  requestToken?: FileEditorRequestToken;
};

/**
 * Fetch a file's content and build its editor state, returning null if the
 * active session or requested file changed while the request was in flight.
 */
export async function fetchFileEditorState({
  client,
  sessionId,
  filePath,
  repo,
  activeSessionIdRef,
  activeRequestRef,
  requestToken = { fileKey: buildRepoScopedItemId(filePath, repo), generation: 0 },
}: FetchFileEditorStateParams): Promise<FileEditorState | null> {
  const requestMatches = () =>
    !activeRequestRef ||
    (activeRequestRef.current?.fileKey === requestToken.fileKey &&
      activeRequestRef.current.generation === requestToken.generation);
  const isCurrent = () => activeSessionIdRef.current === sessionId && requestMatches();
  const response = await requestFileContent(client, sessionId, filePath, repo);
  if (!isCurrent()) return null;
  const state = await buildFileEditorState(filePath, response, repo);
  if (!isCurrent()) return null;
  return state;
}

function panelParamsMatchFile(
  params: Record<string, unknown> | undefined,
  path: string,
  repo?: string,
) {
  if (params?.path !== path) return false;
  let panelRepo: string | undefined;
  if (typeof params.repo === "string") {
    panelRepo = params.repo;
  } else if (typeof params.repositoryName === "string") {
    panelRepo = params.repositoryName;
  }
  return panelRepo === repo;
}

export function isFileEditorPanelAlreadyRestored(
  dockApi: DockApi,
  path: string,
  repo?: string,
): boolean {
  if (!dockApi) return false;
  const itemId = buildRepoScopedItemId(path, repo);
  if (dockApi.getPanel(`file:${itemId}`)) return true;
  const legacyPanel = dockApi.getPanel(`file:${path}`);
  const legacyParams = legacyPanel?.params as Record<string, unknown> | undefined;
  if (legacyPanel && panelParamsMatchFile(legacyParams, path, repo)) return true;
  const previewParams = dockApi.getPanel(PREVIEW_FILE_EDITOR_ID)?.params as
    | Record<string, unknown>
    | undefined;
  return (
    previewParams?.previewItemId === itemId ||
    (previewParams?.previewItemId === path && panelParamsMatchFile(previewParams, path, repo))
  );
}

export function isRestoreWriteCurrent(
  restoredSessionId: string | null,
  activeSessionId: string,
  activeSessionIdRef: MutableRefObject<string | null>,
): boolean {
  return restoredSessionId === activeSessionId && activeSessionIdRef.current === activeSessionId;
}

export function getPreviewItemIdToRemoveOnReplace(
  dockApi: DockApi,
  nextItemId: string,
): string | null {
  if (!dockApi) return null;
  const previewParams = dockApi.getPanel(PREVIEW_FILE_EDITOR_ID)?.params as
    | Record<string, unknown>
    | undefined;
  const previousItemId =
    typeof previewParams?.previewItemId === "string" ? previewParams.previewItemId : null;
  if (!previousItemId || previousItemId === nextItemId) return null;
  if (previewParams?.promoted === true) return null;
  if (dockApi.getPanel(`file:${previousItemId}`)) return null;
  if (dockApi.getPanel(`file:${nextItemId}`)) return null;
  return previousItemId;
}
