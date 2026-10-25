"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getRoutingPreview } from "@/lib/api/domains/office-extended-api";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";
import { t } from "@/lib/i18n";

export type UseRoutingPreviewResult = {
  agents: AgentRoutePreview[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_PREVIEW: AgentRoutePreview[] = [];

export function useRoutingPreview(workspaceName: string | null): UseRoutingPreviewResult {
  const agents = useAppStore((s) =>
    workspaceName
      ? (s.office.routing.preview.byWorkspace[workspaceName] ?? EMPTY_PREVIEW)
      : EMPTY_PREVIEW,
  );
  const setRoutingPreview = useAppStore((s) => s.setRoutingPreview);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const activeRefresh = useRef<(() => Promise<void>) | null>(null);
  const requestVersion = useRef(0);

  const refresh = useCallback(async (): Promise<void> => {
    if (!workspaceName || activeRefresh.current !== refresh) return;
    const version = ++requestVersion.current;
    const isCurrent = () => activeRefresh.current === refresh && requestVersion.current === version;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getRoutingPreview(workspaceName);
      if (!isCurrent()) return;
      setRoutingPreview(workspaceName, res.agents ?? []);
    } catch (e) {
      if (!isCurrent()) return;
      setError(e instanceof Error ? e.message : t("office:failedToLoadRoutingPreview"));
    } finally {
      if (isCurrent()) setIsLoading(false);
    }
  }, [workspaceName, setRoutingPreview]);

  useLayoutEffect(() => {
    activeRefresh.current = refresh;
    setIsLoading(false);
    setError(null);
    return () => {
      activeRefresh.current = null;
      requestVersion.current++;
    };
  }, [refresh]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { agents, isLoading, error, refresh };
}
