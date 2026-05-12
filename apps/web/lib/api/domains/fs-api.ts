import { fetchJson, type ApiRequestOptions } from "../client";

export type DirectoryEntry = {
  name: string;
  path: string;
};

export type DirectoryListing = {
  path: string;
  parent: string;
  entries: DirectoryEntry[];
};

/**
 * Lists immediate subdirectories of `path`. When path is empty the backend
 * defaults to $HOME. Hidden directories (starting with ".") are excluded.
 * Used by the folder picker for repo-less tasks.
 */
export async function listDirectory(path: string, options?: ApiRequestOptions) {
  const qs = path ? `?path=${encodeURIComponent(path)}` : "";
  return fetchJson<DirectoryListing>(`/api/v1/fs/list-dir${qs}`, options);
}
