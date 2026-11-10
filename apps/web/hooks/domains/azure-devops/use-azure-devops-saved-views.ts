"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAzureDevOpsSavedViews,
  setAzureDevOpsSavedViews,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";

type Views = AzureDevOpsSavedView[];
type Update = Views | ((current: Views) => Views);

function applyUpdate(current: Views, update: Update): Views {
  return typeof update === "function" ? update(current) : update;
}

function createScope(workspaceId?: string) {
  return {
    workspaceId,
    confirmed: [] as Views,
    pending: [] as Update[],
    read: undefined as Promise<void> | undefined,
    readFailed: false,
    tail: Promise.resolve(),
    active: false,
  };
}

function ensureRead(scope: ReturnType<typeof createScope>): Promise<void> {
  if (!scope.read || scope.readFailed) {
    scope.readFailed = false;
    scope.read = scope.workspaceId
      ? getAzureDevOpsSavedViews(scope.workspaceId, { cache: "no-store" })
          .then((response) => {
            scope.confirmed = response.views ?? [];
          })
          .catch((error: unknown) => {
            scope.readFailed = true;
            throw error;
          })
      : Promise.resolve();
  }
  return scope.read;
}

export function useAzureDevOpsSavedViews(workspaceId?: string) {
  const [views, setViews] = useState<Views>([]);
  const [loading, setLoading] = useState(true);
  const scopeRef = useRef<ReturnType<typeof createScope> | null>(null);
  if (!scopeRef.current || scopeRef.current.workspaceId !== workspaceId) {
    scopeRef.current = createScope(workspaceId);
  }
  const scope = scopeRef.current;
  const publish = useCallback(() => {
    if (scope.active) setViews(scope.pending.reduce(applyUpdate, scope.confirmed));
  }, [scope]);

  useEffect(() => {
    scope.active = true;
    setLoading(true);
    // Failed loads remain empty on screen but cannot supply a write baseline.
    const finishLoad = () => {
      publish();
      if (scope.active) setLoading(false);
    };
    void ensureRead(scope).then(finishLoad, finishLoad);
    return () => {
      scope.active = false;
    };
  }, [scope, publish]);

  const replace = useCallback(
    (update: Update): Promise<void> => {
      if (!scope.workspaceId) return Promise.resolve();
      const capturedWorkspaceId = scope.workspaceId;
      const read = ensureRead(scope);
      scope.pending.push(update);
      publish();
      const result = scope.tail.then(async () => {
        try {
          await read;
          publish();
          const response = await setAzureDevOpsSavedViews(
            capturedWorkspaceId,
            applyUpdate(scope.confirmed, update),
          );
          scope.confirmed = response.views ?? [];
        } finally {
          scope.pending.shift();
          publish();
        }
      });
      // Keep failures observable to the caller while allowing the next write.
      scope.tail = result.catch(() => {});
      return result;
    },
    [scope, publish],
  );

  const remove = useCallback(
    (id: string) => replace((current) => current.filter((view) => view.id !== id)),
    [replace],
  );

  return { views, loading, replace, remove };
}
