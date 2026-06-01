import { queryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { IntegrationConfigStatus } from "@/hooks/domains/integrations/use-integration-availability";

/**
 * Query options for the integrations domain.
 *
 * These options are shared between SSR prefetch and CSR useQuery calls.
 *
 * health(kind)      — wraps the backend's 90s auth-health poller. Uses
 *                     refetchInterval: 90_000 so the UI stays at most one
 *                     cycle stale without a WS push. kind is e.g. "jira",
 *                     "linear", "slack".
 *
 * availability(kind, fetchConfig) — Combined availability check (authed AND
 *                     enabled). Callers supply `fetchConfig` because the
 *                     fetch function is integration-specific (different
 *                     endpoints). `enabled` is gated on the caller passing
 *                     active=true so disabled integrations never probe.
 *
 * enabled(kind)     — Not a real HTTP query — `useIntegrationEnabled` reads
 *                     localStorage synchronously via useSyncExternalStore.
 *                     A queryOptions entry is provided here only so wave 2
 *                     workers can call qc.setQueryData(qk.integrations.enabled(kind), ...)
 *                     from mutations if they ever need to hydrate the cache.
 *                     The queryFn is intentionally a no-op that returns true
 *                     as the default; callers should NEVER await it.
 */
export const integrationsQueryOptions = {
  /**
   * HTTP probe result from the backend 90s auth-health poller.
   *
   * @param kind    Integration kind string, e.g. "jira", "linear", "slack".
   * @param fetchFn The integration-specific fetch function (e.g. getJiraConfig).
   * @param active  Pass false to skip fetching (e.g. when the user toggle is off).
   */
  health: (
    kind: string,
    fetchFn: () => Promise<IntegrationConfigStatus | null>,
    active: boolean = true,
  ) =>
    queryOptions({
      queryKey: qk.integrations.health(kind),
      queryFn: fetchFn,
      // Mirror the backend poller cadence so the UI is at most ~one cycle stale.
      refetchInterval: 90_000,
      // Do not auto-refetch on reconnect — the WS bridge will invalidate
      // volatile queries; integrations health is low-frequency and cheap.
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      enabled: active,
    }),

  /**
   * Combined availability: the backend reported healthy credentials AND the
   * install-wide user toggle is on.
   *
   * Callers supply `fetchFn` because each integration uses a different endpoint.
   * `active` should be set to `enabled && loaded` from useIntegrationEnabled.
   *
   * @param kind    Integration kind string for the cache key.
   * @param fetchFn Integration-specific config fetcher.
   * @param active  Pass false when the toggle is off — skips fetching.
   */
  availability: (
    kind: string,
    fetchFn: () => Promise<IntegrationConfigStatus | null>,
    active: boolean = true,
  ) =>
    queryOptions({
      queryKey: qk.integrations.availability(kind),
      queryFn: fetchFn,
      // Match the backend poller cadence. This query is the "is integration
      // available?" signal that drives sidebar links and integration panels.
      refetchInterval: 90_000,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      enabled: active,
      // Derive the boolean availability from raw config shape.
      select: (cfg: IntegrationConfigStatus | null): boolean => !!cfg?.hasSecret && !!cfg?.lastOk,
    }),

  /**
   * Install-wide on/off toggle backed by localStorage, NOT an HTTP endpoint.
   *
   * This queryOptions entry exists only so wave 2 workers can call
   * qc.setQueryData(qk.integrations.enabled(kind), value) from mutations.
   * Do NOT use useQuery with this options object — use useIntegrationEnabled
   * directly, which reads localStorage synchronously via useSyncExternalStore.
   *
   * queryFn is a no-op that always resolves to true (the default state).
   */
  enabled: (kind: string) =>
    queryOptions({
      queryKey: qk.integrations.enabled(kind),
      queryFn: () => Promise.resolve(true as boolean),
      // Never stale — localStorage is updated synchronously.
      staleTime: Infinity,
      // Never auto-refetch — localStorage is not an HTTP resource.
      refetchInterval: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    }),
} as const;
