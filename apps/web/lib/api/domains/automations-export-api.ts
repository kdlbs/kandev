import { fetchBlob } from "../client";

const BASE = (workspaceId: string) => `/api/v1/workspaces/${workspaceId}/automations/export`;

/** Fetches the workspace's automations as a zip archive (one file per automation). */
export async function exportAutomationsZip(workspaceId: string): Promise<Blob> {
  return fetchBlob(`${BASE(workspaceId)}/zip`);
}
