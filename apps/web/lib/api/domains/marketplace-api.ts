import { fetchJson, mutationInit, type ApiRequestOptions } from "../client";
import type { MarketplaceCatalog, MarketplaceSource } from "@/lib/types/plugins";

const BASE = "/api/plugins/marketplace";

// CatalogQuery is the filter/sort applied server-side to the merged catalog.
export type CatalogQuery = {
  q?: string;
  category?: string;
  sort?: "stars" | "name" | "recent";
};

function toQueryString(query?: CatalogQuery): string {
  if (!query) return "";
  const params = new URLSearchParams();
  if (query.q) params.set("q", query.q);
  if (query.category) params.set("category", query.category);
  if (query.sort) params.set("sort", query.sort);
  const s = params.toString();
  return s ? `?${s}` : "";
}

// getMarketplaceCatalog fetches the merged catalog across every enabled
// source (GET /api/plugins/marketplace), filtered/sorted by the query. Each
// entry is annotated with its install_state; degraded sources are reported in
// `sources` with healthy=false and omit their entries.
export async function getMarketplaceCatalog(
  query?: CatalogQuery,
  options?: ApiRequestOptions,
): Promise<MarketplaceCatalog> {
  const res = await fetchJson<MarketplaceCatalog>(`${BASE}${toQueryString(query)}`, {
    ...options,
    cache: "no-store",
  });
  return { plugins: res.plugins ?? [], sources: res.sources ?? [] };
}

// listMarketplaceSources returns every configured source, built-in first
// (GET /api/plugins/marketplace/sources).
export async function listMarketplaceSources(
  options?: ApiRequestOptions,
): Promise<MarketplaceSource[]> {
  const res = await fetchJson<{ sources?: MarketplaceSource[] }>(`${BASE}/sources`, options);
  return res.sources ?? [];
}

// addMarketplaceSource registers a new operator source
// (POST /api/plugins/marketplace/sources). Throws ApiError on an invalid or
// duplicate URL (400).
export async function addMarketplaceSource(
  name: string,
  url: string,
  options?: ApiRequestOptions,
): Promise<MarketplaceSource> {
  return fetchJson<MarketplaceSource>(`${BASE}/sources`, {
    ...options,
    init: mutationInit("POST", options, JSON.stringify({ name, url })),
  });
}

// updateMarketplaceSource renames and/or enables/disables a source
// (PATCH /api/plugins/marketplace/sources/:id).
export async function updateMarketplaceSource(
  id: string,
  patch: { name?: string; enabled?: boolean },
  options?: ApiRequestOptions,
): Promise<MarketplaceSource> {
  return fetchJson<MarketplaceSource>(`${BASE}/sources/${encodeURIComponent(id)}`, {
    ...options,
    init: mutationInit("PATCH", options, JSON.stringify(patch)),
  });
}

// deleteMarketplaceSource removes a non-builtin source
// (DELETE /api/plugins/marketplace/sources/:id). Throws ApiError 409 for the
// built-in official source.
export async function deleteMarketplaceSource(
  id: string,
  options?: ApiRequestOptions,
): Promise<{ deleted: boolean }> {
  return fetchJson<{ deleted: boolean }>(`${BASE}/sources/${encodeURIComponent(id)}`, {
    ...options,
    init: mutationInit("DELETE", options),
  });
}

// refreshMarketplace drops the backend's cached source documents so the next
// catalog fetch re-hits every source (POST /api/plugins/marketplace/refresh).
export async function refreshMarketplace(
  options?: ApiRequestOptions,
): Promise<{ refreshed: boolean }> {
  return fetchJson<{ refreshed: boolean }>(`${BASE}/refresh`, {
    ...options,
    init: mutationInit("POST", options),
  });
}
