import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/client";

/**
 * Returns true for HTTP 401 / 403 responses thrown as ApiError.
 * The retry function uses this to avoid pointlessly retrying auth failures.
 */
export function isAuthError(err: unknown): boolean {
  return err instanceof ApiError && (err.status === 401 || err.status === 403);
}

/**
 * Creates a new QueryClient with project-wide defaults:
 * - staleTime 30s — WS bridge owns freshness, not polling.
 * - gcTime 5m — keep inactive queries warm across route transitions.
 * - refetchOnWindowFocus/Reconnect false — WS bridge handles explicit refresh.
 * - retry: skip auth errors; max 2 attempts for transient failures.
 * - mutations: no retry — mutations are not idempotent by default.
 */
export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
        retry: (failureCount, err) => !isAuthError(err) && failureCount < 2,
      },
      mutations: {
        retry: 0,
      },
    },
  });
}

/**
 * Browser-singleton QueryClient (Next 15/16 pattern).
 * - On the server: a new instance is created per request (no module-level sharing).
 * - In the browser: a single instance is lazily created and reused.
 *
 * Callers that need a fresh client (e.g. server-side prefetch in page components)
 * should call makeQueryClient() directly.
 */
let browserClient: QueryClient | undefined;

export function getBrowserQueryClient(): QueryClient {
  if (typeof window === "undefined") {
    // Server: always return a fresh instance to avoid cross-request state leaks.
    return makeQueryClient();
  }
  browserClient ??= makeQueryClient();
  return browserClient;
}
