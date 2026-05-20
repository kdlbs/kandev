import { fetchJson, type ApiRequestOptions } from "../client";
import type { Skill } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

/**
 * Normalises the on-wire snake_case Skill shape into the camelCase
 * `Skill` type the rest of the frontend expects. The backend marshals
 * `models.Skill` directly which yields snake_case keys; without this
 * mapper the new system-skill fields (`is_system`, `system_version`,
 * `default_for_roles`) would be invisible to React selectors.
 */
function normalizeSkill(raw: unknown): Skill {
  const r = (raw ?? {}) as Record<string, unknown>;
  const pick = <T>(snake: string, camel: string): T | undefined =>
    (r[snake] ?? r[camel]) as T | undefined;
  return {
    id: String(r.id ?? ""),
    workspaceId: String(pick<string>("workspace_id", "workspaceId") ?? ""),
    name: String(r.name ?? ""),
    slug: String(r.slug ?? ""),
    description: (r.description as string | undefined) ?? "",
    sourceType: (pick<string>("source_type", "sourceType") ?? "inline") as Skill["sourceType"],
    sourceLocator: pick<string>("source_locator", "sourceLocator"),
    content: (r.content as string | undefined) ?? "",
    fileInventory: normalizeStringArray(pick<unknown>("file_inventory", "fileInventory")),
    createdByAgentProfileId: pick<string>("created_by_agent_profile_id", "createdByAgentProfileId"),
    isSystem: Boolean(pick<unknown>("is_system", "isSystem") ?? false),
    systemVersion: pick<string>("system_version", "systemVersion"),
    defaultForRoles: normalizeStringArray(pick<unknown>("default_for_roles", "defaultForRoles")),
    createdAt: String(pick<string>("created_at", "createdAt") ?? ""),
    updatedAt: String(pick<string>("updated_at", "updatedAt") ?? ""),
  };
}

/**
 * Accepts a value that's either already a `string[]`, or a JSON-
 * encoded array string (the way the office DB stores `default_for_roles`
 * and `file_inventory`). Returns undefined for anything that can't be
 * coerced.
 */
function normalizeStringArray(value: unknown): string[] | undefined {
  if (Array.isArray(value)) {
    return value.filter((s): s is string => typeof s === "string");
  }
  if (typeof value !== "string" || value.trim() === "") return undefined;
  try {
    const parsed = JSON.parse(value);
    if (Array.isArray(parsed)) {
      return parsed.filter((s): s is string => typeof s === "string");
    }
  } catch {
    // fall through
  }
  return undefined;
}

export async function listSkills(workspaceId: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ skills: unknown[] }>(
    `${BASE}/workspaces/${workspaceId}/skills`,
    options,
  );
  return { skills: (res.skills ?? []).map(normalizeSkill) };
}

export async function createSkill(
  workspaceId: string,
  data: Partial<Skill>,
  options?: ApiRequestOptions,
) {
  const res = await fetchJson<{ skill: unknown }>(`${BASE}/workspaces/${workspaceId}/skills`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
  return { skill: normalizeSkill(res.skill) };
}

export async function getSkill(id: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ skill: unknown }>(`${BASE}/skills/${id}`, options);
  return { skill: normalizeSkill(res.skill) };
}

export async function updateSkill(id: string, data: Partial<Skill>, options?: ApiRequestOptions) {
  const res = await fetchJson<{ skill: unknown }>(`${BASE}/skills/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
  return { skill: normalizeSkill(res.skill) };
}

export function deleteSkill(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/skills/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export async function importSkill(
  workspaceId: string,
  source: string,
  options?: ApiRequestOptions,
) {
  const res = await fetchJson<{ skills: unknown[]; warnings: string[] }>(
    `${BASE}/workspaces/${workspaceId}/skills/import`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify({ source }), ...options?.init },
    },
  );
  return { ...res, skills: (res.skills ?? []).map(normalizeSkill) };
}

export function getSkillFile(skillId: string, path: string, options?: ApiRequestOptions) {
  return fetchJson<{ path: string; content: string }>(
    `${BASE}/skills/${skillId}/files?path=${encodeURIComponent(path)}`,
    options,
  );
}
