import { fetchJson, type ApiRequestOptions } from "../client";

/** A node in the organization tree: a department, a team, or a personal unit. */
export type OrgUnit = {
  id: string;
  org_id: string;
  parent_id: string;
  kind: "root" | "standard" | "personal";
  owner_user_id?: string;
  name: string;
  path: string;
  created_at: string;
  updated_at: string;
};

export type UnitMember = {
  unit_id: string;
  user_id: string;
  role: string;
  added_by?: string;
};

export async function listUnits(options?: ApiRequestOptions) {
  return fetchJson<{ units: OrgUnit[] | null; total: number }>("/api/v1/units", options);
}

export async function listUnitMembers(unitId: string, options?: ApiRequestOptions) {
  return fetchJson<{ members: UnitMember[] | null; total: number }>(
    `/api/v1/units/${unitId}/members`,
    options,
  );
}

export async function createUnit(parentId: string, name: string, options?: ApiRequestOptions) {
  return fetchJson<OrgUnit>("/api/v1/units", {
    ...options,
    init: { method: "POST", body: JSON.stringify({ parent_id: parentId, name }), ...options?.init },
  });
}

export async function renameUnit(unitId: string, name: string, options?: ApiRequestOptions) {
  return fetchJson<OrgUnit>(`/api/v1/units/${unitId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify({ name }), ...options?.init },
  });
}

export async function moveUnit(unitId: string, parentId: string, options?: ApiRequestOptions) {
  return fetchJson<OrgUnit>(`/api/v1/units/${unitId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify({ parent_id: parentId }), ...options?.init },
  });
}

export async function deleteUnit(unitId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/units/${unitId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export async function setUnitMember(
  unitId: string,
  userId: string,
  role: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/units/${unitId}/members/${userId}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify({ role }), ...options?.init },
  });
}

export async function removeUnitMember(
  unitId: string,
  userId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/units/${unitId}/members/${userId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

/** Moves a workspace to another unit, which is the only way to change who reaches it. */
export async function placeWorkspace(
  workspaceId: string,
  unitId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ id: string; unit_id: string }>(`/api/v1/workspaces/${workspaceId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify({ unit_id: unitId }), ...options?.init },
  });
}

/**
 * Orders units so a parent always precedes its children, and returns the depth
 * of each. The backend orders by materialized path, which already gives this;
 * depth is derived here so the tree renders without a second walk.
 */
export function withDepth(units: OrgUnit[]): Array<OrgUnit & { depth: number }> {
  return units.map((unit) => ({
    ...unit,
    depth: Math.max(0, unit.path.split("/").filter(Boolean).length - 1),
  }));
}
