import { fetchJson } from "../client";
export type OrchestratorRole = { id: string; name: string; icon?: string; instructions: string };
export type OrchestratorConfiguration = {
  role_id: string;
  profile_id: string;
  executor_preference: string;
  context: string;
};
export type Orchestrator = OrchestratorConfiguration & {
  name: string;
  icon?: string;
  instructions: string;
  id: string;
  workspace_id: string;
  status: string;
};
export type OrchestrationProfile = { id: string; name: string; agent_id: string };
const base = "/api/v1/orchestration";
const workspace = (id: string) => `${base}/workspaces/${encodeURIComponent(id)}`;
const instances = (id: string) => `${workspace(id)}/orchestrators`;
const json = (method: string, body?: unknown) => ({
  init: { method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) },
});
export const listOrchestrators = (ws: string) =>
  fetchJson<{ orchestrators: Orchestrator[] }>(instances(ws));
export const getOrchestrator = (ws: string, id: string) =>
  fetchJson<Orchestrator>(`${instances(ws)}/${encodeURIComponent(id)}`);
export const saveOrchestrator = (
  ws: string,
  id: string | undefined,
  body: OrchestratorConfiguration,
) =>
  fetchJson<Orchestrator>(
    `${instances(ws)}${id ? `/${encodeURIComponent(id)}` : ""}`,
    json(id ? "PUT" : "POST", body),
  );
export const deleteOrchestrator = (ws: string, id: string) =>
  fetchJson(`${instances(ws)}/${encodeURIComponent(id)}`, json("DELETE"));
export const setOrchestratorStatus = (ws: string, id: string, status: string) =>
  fetchJson(`${instances(ws)}/${encodeURIComponent(id)}/status`, json("POST", { status }));
export const openOrchestratorConversation = (ws: string, id: string) =>
  fetchJson<{ task_id: string }>(
    `${instances(ws)}/${encodeURIComponent(id)}/conversation`,
    json("POST"),
  );
export const listOrchestrationProfiles = (ws: string) =>
  fetchJson<{ profiles: OrchestrationProfile[] }>(`${workspace(ws)}/profiles`);
export const listOrchestratorRoles = () =>
  fetchJson<{ roles: OrchestratorRole[] }>(`${base}/roles`);
export const saveOrchestratorRole = (role: Omit<OrchestratorRole, "id"> & { id?: string }) =>
  fetchJson<OrchestratorRole>(
    `${base}/roles${role.id ? `/${encodeURIComponent(role.id)}` : ""}`,
    json(role.id ? "PUT" : "POST", role),
  );
export const deleteOrchestratorRole = (id: string) =>
  fetchJson(`${base}/roles/${encodeURIComponent(id)}`, json("DELETE"));
export const orchestratorsHref = (ws: string) =>
  `/settings/workspaces/${encodeURIComponent(ws)}/orchestration`;
export const orchestratorHref = (ws: string, id: string) =>
  `${orchestratorsHref(ws)}/${encodeURIComponent(id)}`;
export const orchestratorConversationHref = (ws: string, id: string, task: string) =>
  `/workspace/conversations/${encodeURIComponent(task)}?workspaceId=${encodeURIComponent(ws)}&orchestratorId=${encodeURIComponent(id)}`;

export const listOrchestratedTasks = (ws: string, id: string) =>
  fetchJson<{ tasks: { id: string; title: string; state: string }[] }>(
    `${instances(ws)}/${encodeURIComponent(id)}/tasks`,
  );
export function selectedExecutor(raw: string): string {
  try {
    return JSON.parse(raw || "{}").executor_profile_id || "";
  } catch {
    return "";
  }
}

export const coordinatorHref = (workspaceId: string, orchestratorId?: string) =>
  `/workspaces/${encodeURIComponent(workspaceId)}/coordinator${orchestratorId !== undefined ? `?orchestratorId=${encodeURIComponent(orchestratorId)}` : ""}`;
