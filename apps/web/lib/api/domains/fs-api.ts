import { fetchJson, type ApiRequestOptions } from "../client";

export type DirectoryEntry = {
  name: string;
  path: string;
};

export type DirectoryListing = {
  path: string;
  parent: string;
  entries: DirectoryEntry[];
  choosable: boolean;
};

export type ListDirectoryOptions = ApiRequestOptions & {
  /** Reveal entries whose name begins with a dot. Defaults to the backend's
   * current behavior, which omits them. */
  includeHidden?: boolean;
};

/**
 * Lists immediate subdirectories of `path`. When path is empty the backend
 * defaults to $HOME. Hidden directories (starting with ".") are excluded unless
 * `includeHidden` is set, so the default request is unchanged for a caller that
 * does not ask for them. Used by every directory browser.
 */
export async function listDirectory(path: string, options?: ListDirectoryOptions) {
  const query = new URLSearchParams();
  if (path) query.set("path", path);
  // Only the exact value the backend accepts is sent, so a stored preference can
  // never turn into a request the endpoint rejects.
  if (options?.includeHidden) query.set("include_hidden", "true");
  const qs = query.toString();
  return fetchJson<DirectoryListing>(`/api/v1/fs/list-dir${qs ? `?${qs}` : ""}`, options);
}

export async function createDirectory(
  parentPath: string,
  name: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<DirectoryListing>("/api/v1/fs/create-dir", {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ parent_path: parentPath, name }),
      ...(options?.init ?? {}),
    },
  });
}
