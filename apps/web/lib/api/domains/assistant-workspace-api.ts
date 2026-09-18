import { fetchJson } from "../client";
import type { AssistantPage } from "./assistant-types";
import type {
  WorkspaceGrant,
  WorkspaceGrantEvent,
  WorkspaceGrantRequest,
  WorkspaceExportReceipt,
  WorkspaceReceiver,
} from "./assistant-workspace-types";
const base = "/api/v1/orchestration/assistant";
const link = (id: string) => `${base}/workspace-links/${encodeURIComponent(id)}`;
const json = (method: string, body: unknown) => ({ init: { method, body: JSON.stringify(body) } });
export type WorkspacePages = {
  links: WorkspaceGrant;
  options: { id: string; name: string };
  events: WorkspaceGrantEvent;
  exports: WorkspaceExportReceipt;
};
export function getWorkspacePage<K extends keyof WorkspacePages>(
  kind: K,
  id: string,
  after: string,
  signal?: AbortSignal,
) {
  const path = kind === "events" ? `${link(id)}/events` : `${base}/workspace-${kind}`;
  return fetchJson<AssistantPage<WorkspacePages[K]>>(
    `${path}?${new URLSearchParams({ after, limit: "25" })}`,
    { init: { signal } },
  );
}
export const getWorkspaceReceiver = () =>
  fetchJson<{ receiver: WorkspaceReceiver }>(`${base}/workspace-options?limit=1`);
export const saveWorkspaceGrant = (id: string, request: WorkspaceGrantRequest) =>
  fetchJson<WorkspaceGrant>(link(id), json("PUT", request));
export const revokeWorkspaceGrant = (id: string, binding: number, revision: number) =>
  fetchJson(
    link(id),
    json("DELETE", { expected_binding_version: binding, expected_revision: revision }),
  );
export const forgetWorkspaceHandoffs = (id: string, binding: number, revision: number) =>
  fetchJson(
    `${link(id)}/forget`,
    json("POST", { expected_binding_version: binding, expected_revision: revision }),
  );
