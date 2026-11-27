import type { WebSocketClient } from "@/lib/ws/client";
import { requestFileTree } from "@/lib/ws/workspace-files";
import type { FileTreeResponse } from "@/lib/types/backend";
import type { TreeLoadOwner } from "./file-browser-tree-loader";

const requests = new WeakMap<TreeLoadOwner, Map<string, Promise<FileTreeResponse>>>();

export function readOwnedTree(
  client: WebSocketClient,
  owner: TreeLoadOwner,
  path: string,
  isCurrent: () => boolean,
): Promise<FileTreeResponse> {
  let pending = requests.get(owner);
  if (!pending) {
    pending = new Map();
    requests.set(owner, pending);
  }
  const existing = pending.get(path);
  if (existing) return existing;
  const response = requestFileTree(client, owner.sessionId, path, 1)
    .then((value) => {
      if (!isCurrent()) {
        // i18n-exempt: Internal cancellation reason, not displayed as product copy.
        throw new DOMException("Tree load superseded", "AbortError");
      }
      return value;
    })
    .finally(() => {
      if (pending.get(path) === response) pending.delete(path);
    });
  pending.set(path, response);
  return response;
}
