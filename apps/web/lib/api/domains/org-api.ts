import { fetchJson, type ApiRequestOptions } from "../client";
import type { CurrentOrgResponse, ListOrgsResponse, Org, OrgStatus } from "@/lib/types/org";

export async function getCurrentOrg(options?: ApiRequestOptions) {
  return fetchJson<CurrentOrgResponse>("/api/v1/orgs/current", options);
}

export async function listOrgs(options?: ApiRequestOptions) {
  return fetchJson<ListOrgsResponse>("/api/v1/instance/orgs", options);
}

export async function createOrg(name: string, options?: ApiRequestOptions) {
  return fetchJson<Org>("/api/v1/instance/orgs", {
    ...options,
    init: { method: "POST", body: JSON.stringify({ name }), ...(options?.init ?? {}) },
  });
}

export async function updateOrg(
  id: string,
  patch: { name?: string; status?: OrgStatus },
  options?: ApiRequestOptions,
) {
  return fetchJson<Org>(`/api/v1/instance/orgs/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(patch), ...(options?.init ?? {}) },
  });
}

/** Deleting requires typing the org's slug verbatim. */
export async function deleteOrg(id: string, slug: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/instance/orgs/${id}`, {
    ...options,
    init: { method: "DELETE", body: JSON.stringify({ slug }), ...(options?.init ?? {}) },
  });
}

export async function createOrgAdmin(
  id: string,
  admin: { email: string; password: string; display_name: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/instance/orgs/${id}/admins`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(admin), ...(options?.init ?? {}) },
  });
}
