import type { WebSocketClient } from './client';
import type { FileTreeResponse, FileContentResponse } from '@/lib/types/backend';

/**
 * Request file tree from the backend
 */
export async function requestFileTree(
  client: WebSocketClient,
  taskId: string,
  path: string = '',
  depth: number = 1
): Promise<FileTreeResponse> {
  return client.request<FileTreeResponse>('workspace.tree.get', {
    task_id: taskId,
    path,
    depth,
  });
}

/**
 * Request file content from the backend
 */
export async function requestFileContent(
  client: WebSocketClient,
  taskId: string,
  path: string
): Promise<FileContentResponse> {
  return client.request<FileContentResponse>('workspace.file.get', {
    task_id: taskId,
    path,
  });
}
