"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAzureDevOpsSavedViews,
  setAzureDevOpsSavedViews,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";

export function useAzureDevOpsSavedViews(workspaceId?: string) {
  const [views, setViews] = useState<AzureDevOpsSavedView[]>([]);
  const [loading, setLoading] = useState(true);
  const viewsRef = useRef(views);
  viewsRef.current = views;

  useEffect(() => {
    if (!workspaceId) {
      setViews([]);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    getAzureDevOpsSavedViews(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) setViews(response.views ?? []);
      })
      .catch(() => {
        if (!cancelled) setViews([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  const replace = useCallback(
    async (next: AzureDevOpsSavedView[]) => {
      if (!workspaceId) return;
      const previous = viewsRef.current;
      setViews(next);
      try {
        const response = await setAzureDevOpsSavedViews(workspaceId, next);
        setViews(response.views ?? []);
      } catch (error) {
        setViews(previous);
        throw error;
      }
    },
    [workspaceId],
  );

  const remove = useCallback(
    (id: string) => replace(viewsRef.current.filter((view) => view.id !== id)),
    [replace],
  );

  return { views, loading, replace, remove };
}
