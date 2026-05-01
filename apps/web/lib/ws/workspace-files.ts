import type { WebSocketClient } from "./client";
import type {
  FileTreeResponse,
  FileContentResponse,
  FileSearchResponse,
} from "@/lib/types/backend";

/**
 * Request file tree from the backend
 */
export async function requestFileTree(
  client: WebSocketClient,
  sessionId: string,
  path: string = "",
  depth: number = 1,
): Promise<FileTreeResponse> {
  return client.request<FileTreeResponse>("workspace.tree.get", {
    session_id: sessionId,
    path,
    depth,
  });
}

/**
 * Request file content from the backend.
 * `repo` is the multi-repo subpath (e.g. "kandev"); omit for single-repo
 * workspaces. When set, `path` is interpreted relative to that repo.
 */
export async function requestFileContent(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  repo?: string,
): Promise<FileContentResponse> {
  return client.request<FileContentResponse>("workspace.file.get", {
    session_id: sessionId,
    path,
    ...(repo ? { repo } : {}),
  });
}

/**
 * Request file content at a specific git ref (branch, commit, HEAD, etc.)
 * Used for diff expansion to fetch old/base version of a file.
 * `repo` scopes to a per-repo subdirectory for multi-repo workspaces.
 */
export async function requestFileContentAtRef(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  ref: string,
  repo?: string,
): Promise<FileContentResponse> {
  return client.request<FileContentResponse>("workspace.file.get_at_ref", {
    session_id: sessionId,
    path,
    ref,
    ...(repo ? { repo } : {}),
  });
}

/**
 * Search for files in the workspace matching a query
 */
export async function searchWorkspaceFiles(
  client: WebSocketClient,
  sessionId: string,
  query: string,
  limit: number = 20,
): Promise<FileSearchResponse> {
  return client.request<FileSearchResponse>("workspace.files.search", {
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
  resolution?: "applied" | "overwritten";
  error?: string;
};

/**
 * Update file content using a diff. `repo` scopes to a per-repo subdirectory
 * for multi-repo workspaces; omit for single-repo.
 */
export async function updateFileContent(
  client: WebSocketClient,
  sessionId: string,
  params: {
    path: string;
    diff: string;
    originalHash: string;
    desiredContent?: string;
    repo?: string;
  },
): Promise<FileUpdateResponse> {
  return client.request<FileUpdateResponse>("workspace.file.update", {
    session_id: sessionId,
    path: params.path,
    diff: params.diff,
    original_hash: params.originalHash,
    ...(params.desiredContent !== undefined && {
      desired_content: params.desiredContent,
    }),
    ...(params.repo ? { repo: params.repo } : {}),
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
 * Create a new file in the workspace. `repo` scopes the create to a per-repo
 * subdirectory for multi-repo task workspaces; omit for single-repo.
 */
export async function createFile(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  repo?: string,
): Promise<FileCreateResponse> {
  return client.request<FileCreateResponse>("workspace.file.create", {
    session_id: sessionId,
    path,
    ...(repo ? { repo } : {}),
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
 * Delete a file from the workspace. `repo` scopes the delete to a per-repo
 * subdirectory for multi-repo task workspaces; omit for single-repo.
 */
export async function deleteFile(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  repo?: string,
): Promise<FileDeleteResponse> {
  return client.request<FileDeleteResponse>("workspace.file.delete", {
    session_id: sessionId,
    path,
    ...(repo ? { repo } : {}),
  });
}

/**
 * File rename response from backend
 */
export type FileRenameResponse = {
  old_path: string;
  new_path: string;
  success: boolean;
  error?: string;
};

/**
 * Rename a file or directory in the workspace. `repo` scopes BOTH oldPath and
 * newPath to a per-repo subdirectory for multi-repo task workspaces; omit
 * for single-repo. Cross-repo moves aren't supported.
 */
export async function renameFile(
  client: WebSocketClient,
  sessionId: string,
  oldPath: string,
  newPath: string,
  repo?: string,
): Promise<FileRenameResponse> {
  return client.request<FileRenameResponse>("workspace.file.rename", {
    session_id: sessionId,
    old_path: oldPath,
    new_path: newPath,
    ...(repo ? { repo } : {}),
  });
}
