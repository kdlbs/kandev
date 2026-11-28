"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";
import { useTranslation } from "react-i18next";

export type RepositorySource = "local" | RemoteRepositoryProvider;

export function useRepositoryPickerSource(
  providerIds: RemoteRepositoryProvider[],
  scopeKey?: string,
) {
  const [source, setSource] = useState<RepositorySource>("local");
  const [unavailableProvider, setUnavailableProvider] = useState<RemoteRepositoryProvider | null>(
    null,
  );
  const restoredScopeRef = useRef<string | null>(null);
  const unavailableSource = source !== "local" && !providerIds.includes(source) ? source : null;

  useEffect(() => {
    if (!unavailableSource) return;
    setUnavailableProvider(unavailableSource);
    setSource("local");
  }, [unavailableSource]);

  useEffect(() => {
    if (!unavailableProvider || !providerIds.includes(unavailableProvider)) return;
    setUnavailableProvider(null);
  }, [providerIds, unavailableProvider]);

  useEffect(() => {
    if (!scopeKey || restoredScopeRef.current === scopeKey || providerIds.length === 0) return;
    restoredScopeRef.current = scopeKey;
    const stored = readStoredRepositorySource(scopeKey);
    if (stored && (stored === "local" || providerIds.includes(stored))) setSource(stored);
  }, [providerIds, scopeKey]);

  const selectSource = useCallback(
    (next: RepositorySource) => {
      setUnavailableProvider(null);
      setSource(next);
      if (scopeKey) sessionStorage.setItem(repositorySourceStorageKey(scopeKey), next);
    },
    [scopeKey],
  );

  return {
    activeSource: unavailableSource ?? source,
    unavailableProvider,
    selectSource,
  };
}

export function RepositorySourceUnavailableNotice({ onRefresh }: { onRefresh: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center justify-between gap-2 px-2 py-3 text-xs text-destructive"
      role="alert"
      data-testid="task-repository-source-unavailable"
    >
      <span className="min-w-0 break-words">{t("task:repositorySelectionUnavailable")}</span>
      <Button type="button" variant="outline" onClick={onRefresh} className="min-h-11 shrink-0">
        {t("task:retry")}
      </Button>
    </div>
  );
}

function repositorySourceStorageKey(scopeKey: string): string {
  return `kandev.task-repository-picker-source:${scopeKey}`;
}

function readStoredRepositorySource(scopeKey: string): RepositorySource | null {
  if (typeof window === "undefined") return null;
  const stored = window.sessionStorage.getItem(repositorySourceStorageKey(scopeKey));
  return stored ? (stored as RepositorySource) : null;
}
