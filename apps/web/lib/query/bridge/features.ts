import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Feature-flags bridge registrar.
 *
 * Feature flags are set at deployment time via backend env vars and are
 * read once per SSR render + once on the client on mount. They do NOT
 * have live WS updates — the backend never pushes flag changes over the
 * WebSocket channel. There is therefore no handler to mirror from
 * lib/ws/handlers/ for this domain.
 *
 * If a future release adds a `features.updated` WS event, wire it here:
 *   ws.on("features.updated", (payload) => {
 *     qc.setQueryData(qk.features(), payload.flags);
 *   });
 *
 * For now this is intentionally a no-op registrar.
 */
export function registerFeaturesBridge(_ws: WebSocketClient, _qc: QueryClient): () => void {
  // No WS handler to register — features are static for the lifetime of
  // a browser session and are seeded from SSR prefetch / initial fetch.
  return () => {
    // no-op cleanup
  };
}
