import { pluginRegistry } from "./registry";
import type { PluginRepositoryProviderRegistration } from "./registry";
import type { RepositoryInspection, RepositoryProviderRegistration } from "./types";

export type InspectedRepositoryProvider = {
  provider: PluginRepositoryProviderRegistration;
  inspection: RepositoryInspection;
};

export function repositoryProviderMatchesURL(
  provider: RepositoryProviderRegistration,
  url: string,
): boolean {
  try {
    return provider.matchesURL(url);
  } catch {
    return false;
  }
}

export function hasRegisteredRepositoryProviderCandidate(url: string): boolean {
  return pluginRegistry
    .getRepositoryProviders()
    .some((provider) => repositoryProviderMatchesURL(provider, url));
}

/**
 * Treats `matchesURL` as a cheap coarse filter only. Provider ownership is
 * established by workspace-scoped structured inspection, so overlapping
 * self-hosted URL shapes cannot silently route to registration order.
 */
export async function inspectRegisteredRepositoryURL(context: {
  workspaceId: string;
  url: string;
  signal: AbortSignal;
}): Promise<InspectedRepositoryProvider | null> {
  const candidates = pluginRegistry
    .getRepositoryProviders()
    .filter((provider) => repositoryProviderMatchesURL(provider, context.url));
  const inspected = await Promise.allSettled(
    candidates.map(async (provider) => ({
      provider,
      inspection: await provider.inspectURL(context),
    })),
  );
  const matches: InspectedRepositoryProvider[] = inspected.flatMap((result) =>
    result.status === "fulfilled" && result.value.inspection
      ? [{ provider: result.value.provider, inspection: result.value.inspection }]
      : [],
  );
  if (matches.length > 1) {
    throw new Error(
      `More than one repository provider recognized this URL: ${matches
        .map((entry) => entry.provider.id)
        .join(", ")}`,
    );
  }
  if (matches[0]) return matches[0];
  const failure = inspected.find(
    (result): result is PromiseRejectedResult => result.status === "rejected",
  );
  if (failure) {
    throw failure.reason instanceof Error ? failure.reason : new Error(String(failure.reason));
  }
  return null;
}
