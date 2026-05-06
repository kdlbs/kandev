/**
 * Server-side typed fetch for Next.js Server Components rendering
 * office pages. Mirrors the client-side `fetchJson` helper but
 * resolves the backend base URL from env on the server (no
 * `window`-based fallback) and skips Next.js's data cache so each
 * navigation produces fresh data — matching the live-data nature
 * of the office surface.
 *
 * Use this in `page.tsx` files (Server Components) only. Client
 * Components stick with `apps/web/lib/api/domains/*` helpers.
 */

import { getBackendConfig } from "@/lib/config";

export type ServerFetchOptions = {
  /** HTTP method. Defaults to GET. */
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  /** JSON body — serialized automatically. */
  body?: unknown;
  /** Extra request headers (Content-Type is set automatically). */
  headers?: Record<string, string>;
  /**
   * Pass `true` to opt into Next.js caching of this request. Default
   * is `false` because office data is live; pages should re-fetch
   * on every server render.
   */
  cache?: boolean;
};

export class ServerFetchError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string,
    public readonly path: string,
  ) {
    super(`server fetch ${path} failed (${status}): ${body}`);
    this.name = "ServerFetchError";
  }
}

const OFFICE_BASE = "/api/v1/office";

/**
 * Awaits a JSON response from the backend's office API. Path is
 * relative to `/api/v1/office` — pass e.g. `"/agents/123/summary"`.
 * Returns the parsed JSON body, typed by the caller's generic.
 *
 * Throws `ServerFetchError` on non-2xx responses; let it bubble so
 * Next.js renders an error boundary rather than swallowing it.
 */
export async function serverFetchOfficeJson<T>(
  path: string,
  options: ServerFetchOptions = {},
): Promise<T> {
  const base = getBackendConfig().apiBaseUrl.replace(/\/$/, "");
  const url = `${base}${OFFICE_BASE}${path.startsWith("/") ? path : `/${path}`}`;
  const init: RequestInit = {
    method: options.method ?? "GET",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
    cache: options.cache ? "force-cache" : "no-store",
  };
  if (options.body !== undefined) {
    init.body = JSON.stringify(options.body);
  }
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new ServerFetchError(res.status, body, path);
  }
  return (await res.json()) as T;
}
