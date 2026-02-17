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

/**
 * File update response from backend
 */
export type FileUpdateResponse = {
  path: string;
  success: boolean;
  new_hash?: string;
  error?: string;
};

/**
 * Update file content using a diff
 */
export async function updateFileContent(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  diff: string,
  originalHash: string
): Promise<FileUpdateResponse> {
  return client.request<FileUpdateResponse>('workspace.file.update', {
    session_id: sessionId,
    path,
    diff,
    original_hash: originalHash,
  });
}

/**
 * File create response from backend
 */
export type FileCreateResponse = {
  path: string;
  success: boolean;
  error?: string;
};

/**
 * Create a new file in the workspace
 */
export async function createFile(
  client: WebSocketClient,
  sessionId: string,
  path: string
): Promise<FileCreateResponse> {
  return client.request<FileCreateResponse>('workspace.file.create', {
    session_id: sessionId,
    path,
  });
}

/**
 * File delete response from backend
 */
export type FileDeleteResponse = {
  path: string;
  success: boolean;
  error?: string;
};

/**
 * Delete a file from the workspace
 */
export async function deleteFile(
  client: WebSocketClient,
  sessionId: string,
  path: string
): Promise<FileDeleteResponse> {
  return client.request<FileDeleteResponse>('workspace.file.delete', {
    session_id: sessionId,
    path,
  });
}

