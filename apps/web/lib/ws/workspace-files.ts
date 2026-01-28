import type { WebSocketClient } from './client';
import type { FileTreeResponse, FileContentResponse, FileSearchResponse } from '@/lib/types/backend';

/**
 * Request file tree from the backend
 */
export async function requestFileTree(
  client: WebSocketClient,
  sessionId: string,
  path: string = '',
  depth: number = 1
): Promise<FileTreeResponse> {
  return client.request<FileTreeResponse>('workspace.tree.get', {
    session_id: sessionId,
    path,
    depth,
  });
}

/**
 * Request file content from the backend
 */
export async function requestFileContent(
  client: WebSocketClient,
  sessionId: string,
  path: string
): Promise<FileContentResponse> {
  return client.request<FileContentResponse>('workspace.file.get', {
    session_id: sessionId,
    path,
  });
}

/**
 * Search for files in the workspace matching a query
 */
export async function searchWorkspaceFiles(
  client: WebSocketClient,
  sessionId: string,
  query: string,
  limit: number = 20
): Promise<FileSearchResponse> {
  return client.request<FileSearchResponse>('workspace.files.search', {
    session_id: sessionId,
    query,
    limit,
  });
}
