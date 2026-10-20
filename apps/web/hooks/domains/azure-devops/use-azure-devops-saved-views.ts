"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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

export function useAzureDevOpsSavedViews(workspaceId?: string) {
  const [views, setViews] = useState<Views>([]);
  const [loading, setLoading] = useState(true);
  const scope = useMemo(
    () => ({
      workspaceId,
      confirmed: [] as Views,
      pending: [] as Update[],
      read: undefined as Promise<void> | undefined,
      tail: Promise.resolve(),
      active: false,
    }),
    [workspaceId],
  );
  const publish = useCallback(() => {
    if (scope.active) setViews(scope.pending.reduce(applyUpdate, scope.confirmed));
  }, [scope]);

  useEffect(() => {
    scope.active = true;
    setLoading(true);
    // The initial read supplies the baseline beneath any admitted updates.
    scope.read ??= scope.workspaceId
      ? getAzureDevOpsSavedViews(scope.workspaceId, { cache: "no-store" })
          .then((response) => {
            scope.confirmed = response.views ?? [];
          })
          .catch(() => {
            scope.confirmed = [];
          })
      : Promise.resolve();
    void scope.read.then(() => {
      publish();
      if (scope.active) setLoading(false);
    });
    return () => {
      scope.active = false;
    };
  }, [scope, publish]);

  const replace = useCallback(
    (update: Update): Promise<void> => {
      if (!scope.workspaceId) return Promise.resolve();
      const capturedWorkspaceId = scope.workspaceId;
      scope.pending.push(update);
      publish();
      const result = scope.tail.then(async () => {
        await scope.read;
        try {
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
