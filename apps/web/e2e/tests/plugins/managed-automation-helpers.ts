import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

type CapabilityApprovalContext = {
  manifest_digest: string;
  approval?: { revision: number };
};

export type ManagedDestination = {
  plugin_id: string;
  instance_key: string;
  revision: number;
  task_id?: string;
  session_id?: string;
};

export async function installAndGrantManagedConversationAccess(
  page: Page,
  apiClient: ApiClient,
  workspaceId: string,
  additionalCapabilityIds: readonly string[] = [],
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
      expected_revision: context.approval?.revision ?? 0,
      manifest_digest: context.manifest_digest,
      capability_ids: [
        "host.v2.read:automations",
        "host.v2.write:automations",
        "host.v2.read:managed_agent_conversations",
        "host.v2.write:managed_agent_conversations",
        ...additionalCapabilityIds,
      ],
      reason: "Exercise managed conversation Host APIs",
      audit_id: `managed-automation-${randomUUID()}`,
    },
  );
  if (!approvalResponse.ok) {
    throw new Error(`Could not grant managed automation access: ${approvalResponse.status}`);
  }
}

export function installAndGrantManagedChatAccess(
  page: Page,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<void> {
  return installAndGrantManagedConversationAccess(page, apiClient, workspaceId, [
    "host.v2.read:messages",
    "host.v2.read:tasks",
    "host.v2.read:sessions",
    "host.v2.read:interactions",
    "host.v2.write:interactions",
    "host.v2.write:execution",
  ]);
}

export async function openManagedChatPage(
  page: Page,
  config: {
    workspaceId: string;
    agentProfileId: string;
    selectedInstanceKey: string;
    instances: Array<{ key: string; label: string }>;
  },
) {
  await page.addInitScript((value) => {
    Object.assign(window, { __e2eManagedChatConfig: value });
  }, config);
  await page.goto("/plugins/e2e-managed-chat");
}

export async function ensureManagedConversation(
  apiClient: ApiClient,
  workspaceId: string,
  instanceKey: string,
  agentProfileId: string,
): Promise<ManagedDestination> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/plugins/${PLUGIN_ID}/actions/managed-conversation-ensure`,
    { workspaceId, body: { instance_key: instanceKey, agent_profile_id: agentProfileId } },
  );
  const body = await response.text();
  if (!response.ok)
    throw new Error(`Could not create managed conversation (${response.status}): ${body}`);
  return JSON.parse(body) as ManagedDestination;
}

export async function listManagedDestinations(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/plugins/workspaces/${encodeURIComponent(workspaceId)}/managed-conversation-destinations`,
  );
  if (!response.ok) throw new Error(`Could not list managed destinations: ${response.status}`);
  return (await response.json()) as {
    destinations: Array<
      ManagedDestination & { plugin_name: string; paused: boolean; unavailable_reason?: string }
    >;
  };
}

export async function seedManagedAutomation(
  apiClient: ApiClient,
  workspaceId: string,
  name: string,
  destination: ManagedDestination,
) {
  const automation = await apiClient.seedAutomation({
    workspaceId,
    name,
    taskMode: "managed_conversation",
    managedDestination: destination,
    prompt: "Report blockers from the workspace.",
  });
  await apiClient.seedTrigger({
    automationId: automation.id,
    type: "scheduled",
    config: { cron_expression: "0 9 * * 1-5", timezone: "UTC" },
    enabled: true,
  });
  return automation;
}
