import { fetchJson } from "../client";
import type { AssistantPage } from "./assistant-types";
import type {
  ImprovementDetail,
  ImprovementEvidence,
  MaintenanceArtifact,
  MaintenanceFile,
  MaintenanceGrant,
  MaintenanceGrantRequest,
  MaintenanceOption,
  MaintenanceRequest,
  MaintenanceSuccess,
} from "./assistant-maintenance-types";
const base = "/api/v1/orchestration/assistant";
const proposal = (id: string) => `${base}/improvements/${encodeURIComponent(id)}`;
const json = (method: string, body: unknown) => ({ init: { method, body: JSON.stringify(body) } });
export const getImprovement = (id: string, signal?: AbortSignal) =>
  fetchJson<ImprovementDetail>(proposal(id), { init: { signal } });
export const getMaintenanceArtifact = (id: string, signal?: AbortSignal) =>
  fetchJson<MaintenanceArtifact>(`${proposal(id)}/artifact`, { init: { signal } });
export const getMaintenanceFile = (id: string, path: string, binding: number, grant: number) =>
  fetchJson<MaintenanceFile>(
    `${proposal(id)}/file?${new URLSearchParams({ path, expected_binding_version: String(binding), grant_revision: String(grant) })}`,
  );
export const saveMaintenanceGrant = (id: string, request: MaintenanceGrantRequest) =>
  fetchJson<MaintenanceGrant>(`${proposal(id)}/grant`, json("PUT", request));
export const revokeMaintenanceGrant = (id: string, binding: number, revision: number) =>
  fetchJson(
    `${proposal(id)}/grant`,
    json("DELETE", { expected_binding_version: binding, expected_revision: revision }),
  );
export const runMaintenance = (id: string, request: MaintenanceRequest) =>
  fetchJson(`${proposal(id)}/maintenance`, json("POST", request));
export const reviewImprovement = (
  id: string,
  binding: number,
  revision: number,
  action: "resolved" | "rejected",
  success?: MaintenanceSuccess,
) =>
  fetchJson(
    `${proposal(id)}/review`,
    json("POST", {
      expected_binding_version: binding,
      expected_revision: revision,
      action,
      evidence: success
        ? {
            source_kind: "task_message",
            task_id: success.task_id,
            session_id: success.session_id,
            source_id: success.source_id,
          }
        : undefined,
    }),
  );
export const reconcileMaintenance = (id: string, binding: number, revision: number) =>
  fetchJson(
    `${proposal(id)}/reconcile`,
    json("POST", { expected_binding_version: binding, expected_revision: revision }),
  );
export type MaintenancePages = {
  options: MaintenanceOption;
  evidence: ImprovementEvidence;
  successes: MaintenanceSuccess;
};
export const getMaintenancePage = <K extends keyof MaintenancePages>(
  kind: K,
  id: string,
  after: string,
  signal?: AbortSignal,
) => {
  const path = kind === "options" ? `${base}/maintenance-options` : `${proposal(id)}/${kind}`;
  return fetchJson<AssistantPage<MaintenancePages[K]>>(
    `${path}?${new URLSearchParams({ after, limit: "25" })}`,
    { init: { signal } },
  );
};
