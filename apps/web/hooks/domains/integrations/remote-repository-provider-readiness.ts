"use client";

import { useEffect, useMemo, useState } from "react";
import type { PluginRepositoryProviderRegistration } from "@/lib/plugins/registry";
import type {
  RemoteRepositoryProviderCatalogEntry,
  RemoteRepositorySourceError,
} from "./use-remote-repositories";

type AvailabilitySnapshot = {
  workspaceId: string;
  providers: PluginRepositoryProviderRegistration[];
  revision: number;
  catalog: RemoteRepositoryProviderCatalogEntry[];
  readyProviders: PluginRepositoryProviderRegistration[];
  sourceErrors: RemoteRepositorySourceError[];
};

const EMPTY_PROVIDERS: PluginRepositoryProviderRegistration[] = [];
const EMPTY_ERRORS: RemoteRepositorySourceError[] = [];

async function checkAvailability(
  provider: PluginRepositoryProviderRegistration,
  workspaceId: string,
  signal: AbortSignal,
): Promise<RemoteRepositoryProviderCatalogEntry> {
  try {
    const status = await provider.getAvailability?.({ workspaceId, signal });
    const ready = status?.configured === true && status.enabled === true && status.tested === true;
    return { provider: provider.id, readiness: ready ? "ready" : "unavailable" };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { provider: provider.id, readiness: "failed", error: new Error(message.slice(0, 512)) };
  }
}

export function usePluginRepositoryAvailability(
  workspaceId: string,
  providers: PluginRepositoryProviderRegistration[],
  revision: number,
) {
  const [snapshot, setSnapshot] = useState<AvailabilitySnapshot | null>(null);
  useEffect(() => {
    if (!workspaceId || providers.length === 0) return;
    const controller = new AbortController();
    const checks = providers.map((provider) =>
      checkAvailability(provider, workspaceId, controller.signal),
    );
    void Promise.all(checks).then((catalog) => {
      if (controller.signal.aborted) return;
      const readyProviders = providers.filter((_, index) => catalog[index]?.readiness === "ready");
      const sourceErrors = catalog.flatMap(({ provider, error }) =>
        error ? [{ provider, error }] : [],
      );
      setSnapshot((previous) => ({
        workspaceId,
        providers,
        revision,
        catalog,
        readyProviders:
          previous?.providers === providers &&
          previous.readyProviders.length === readyProviders.length &&
          readyProviders.every((provider, index) => previous.readyProviders[index] === provider)
            ? previous.readyProviders
            : readyProviders,
        sourceErrors,
      }));
    });
    return () => controller.abort();
  }, [providers, revision, workspaceId]);

  const loadingCatalog = useMemo(
    (): RemoteRepositoryProviderCatalogEntry[] =>
      workspaceId
        ? providers.map((provider) => ({
            provider: provider.id,
            readiness: provider.getAvailability ? "loading" : "unavailable",
          }))
        : [],
    [providers, workspaceId],
  );
  const sameScope = snapshot?.workspaceId === workspaceId && snapshot.providers === providers;
  if (sameScope && snapshot.revision === revision) {
    return { ...snapshot, loading: false };
  }
  return {
    catalog: loadingCatalog,
    readyProviders: sameScope ? snapshot.readyProviders : EMPTY_PROVIDERS,
    sourceErrors: EMPTY_ERRORS,
    loading: loadingCatalog.some((entry) => entry.readiness === "loading"),
  };
}
