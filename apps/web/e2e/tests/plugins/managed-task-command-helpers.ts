import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

type CapabilityApprovalContext = {
  manifest_digest: string;
};

export type PluginTaskCreateInput = {
  idempotency_key: string;
  external_id: string;
  title: string;
  workflow_id: string;
  workflow_step_id: string;
};

export type PluginTaskCreateResult = {
  status: string;
  task_id?: string;
  reason?: string;
};

export async function installAndGrantTaskWrite(
  page: Page,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<void> {
  await installFixturePlugin(page);
  const contextResponse = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${PLUGIN_ID}/capability-approvals?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!contextResponse.ok) {
    throw new Error(`Could not read plugin capability context: ${contextResponse.status}`);
  }
  const context = (await contextResponse.json()) as CapabilityApprovalContext;
  const approvalResponse = await apiClient.rawRequest(
    "PUT",
    `/api/plugins/${PLUGIN_ID}/capability-approvals`,
    {
      workspace_id: workspaceId,
      expected_revision: 0,
      manifest_digest: context.manifest_digest,
      capability_ids: ["host.v2.write:tasks"],
      reason: "Exercise exact task create retry in the managed Host",
      audit_id: `managed-task-create-${randomUUID()}`,
    },
  );
  if (!approvalResponse.ok) {
    throw new Error(`Could not grant exact task creation: ${approvalResponse.status}`);
  }
}

export function createPluginTaskInput(
  title: string,
  workflowId: string,
  workflowStepId: string,
): PluginTaskCreateInput {
  const key = `managed-task-${randomUUID()}`;
  return {
    idempotency_key: key,
    external_id: `managed-task-source-${key}`,
    title,
    workflow_id: workflowId,
    workflow_step_id: workflowStepId,
  };
}

export async function invokeExactTaskCreate(
  apiClient: ApiClient,
  workspaceId: string,
  input: PluginTaskCreateInput,
): Promise<PluginTaskCreateResult> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/plugins/${PLUGIN_ID}/actions/exact-task-create`,
    { workspaceId, body: input },
  );
  const text = await response.text();
  if (!response.ok) throw new Error(`Exact task action failed (${response.status}): ${text}`);
  return JSON.parse(text) as PluginTaskCreateResult;
}

export async function invokePluginTaskTreeDelete(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<Response> {
  return apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/task-tree-delete`, {
    workspaceId,
    body: { task_id: taskId },
  });
}
