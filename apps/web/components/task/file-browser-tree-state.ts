import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useSessionReadScope } from "@/hooks/domains/session/use-session-read";
import { environmentReadIdentity } from "@/lib/state/session-read-coordinator";
import type { FileTreeNode } from "@/lib/types/backend";
import { FileBrowserTreeCache, type FileTreeCacheBinding } from "./file-browser-tree-cache";

const caches = new WeakMap<object, FileBrowserTreeCache>();

export function useFileTreeCacheBinding(environmentOrSessionId: string, resetKey: string) {
  const store = useAppStoreApi();
  const scope = useSessionReadScope();
  const environmentId = useAppStore(
    (state) => state.environmentIdBySessionId[environmentOrSessionId] ?? environmentOrSessionId,
  );
  const restoration = useAppStore((state) => environmentReadIdentity(state, environmentId));
  return useMemo(() => {
    let cache = caches.get(scope);
    if (!cache) {
      cache = new FileBrowserTreeCache();
      caches.set(scope, cache);
    }
    return {
      cache,
      key: JSON.stringify([environmentId, resetKey, restoration]),
      isCurrent: () =>
        scope.current() &&
        (store.getState().environmentIdBySessionId[environmentOrSessionId] ??
          environmentOrSessionId) === environmentId &&
        environmentReadIdentity(store.getState(), environmentId) === restoration &&
        (!store.getState().workspaceRestoration.byEnvironmentId[environmentId] ||
          store.getState().workspaceRestoration.byEnvironmentId[environmentId].status === "ready"),
    };
  }, [environmentId, environmentOrSessionId, resetKey, restoration, scope, store]);
}

function cachedTree(binding?: FileTreeCacheBinding) {
  return binding?.isCurrent() ? binding.cache.get(binding.key) : null;
}

export function useFileTreeState(resetKey: string, binding?: FileTreeCacheBinding) {
  const owner = useMemo(() => ({ resetKey, binding }), [resetKey, binding]);
  const ownerRef = useRef(owner);
  ownerRef.current = owner;
  const [snapshot, setSnapshot] = useState(() => ({ owner, tree: cachedTree(binding) }));
  const tree = snapshot.owner === owner ? snapshot.tree : cachedTree(binding);
  const setTree: Dispatch<SetStateAction<FileTreeNode | null>> = useCallback((update) => {
    const requestedOwner = ownerRef.current;
    setSnapshot((previous) => {
      if (ownerRef.current !== requestedOwner) return previous;
      const current =
        previous.owner === requestedOwner ? previous.tree : cachedTree(requestedOwner.binding);
      return {
        owner: requestedOwner,
        tree: typeof update === "function" ? update(current) : update,
      };
    });
  }, []);
  useEffect(() => {
    if (snapshot.owner === owner && binding?.isCurrent()) {
      binding.cache.set(binding.key, snapshot.tree);
    }
  }, [snapshot, owner, binding]);
  return { tree, setTree };
}
